package service

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ProxyServices struct {
	services map[string]*ProxyService
	parent   *ProxyServices
}

func NewProxyServices(services []*Service) *ProxyServices {
	ret := &ProxyServices{
		services: map[string]*ProxyService{},
	}
	if services != nil {
		for _, service := range services {
			if service == nil {
				continue
			}
			ret.services[service.Name] = NewProxyService(service)
		}
	}

	return ret
}

func (this *ProxyServices) stop() {
	for _, s := range this.services {
		s.stopHealthCheck()
	}
}

func (this *ProxyServices) addParent(parent *ProxyServices) {
	this.parent = parent
}

func (this *ProxyServices) balanceHost(name string) (string, bool) {
	service, exist := this.services[name]
	debug("find balance service", name, exist)
	if !exist {
		if this.parent != nil {
			debug("try parent proxyServices")
			return this.parent.balanceHost(name)
		}
		return "", false
	}
	return service.balanceHost()
}

type ProxyService struct {
	mux              sync.Mutex
	hostsCount       int
	requestTimes     int
	requestSequences []int
	indexWeight      []int
	indexAlive       []bool
	hosts            []string
	checks           []*ProxyCheck
}

func NewProxyService(service *Service) *ProxyService {
	ret := &ProxyService{
		hostsCount:       len(service.Hosts),
		requestTimes:     0,
		requestSequences: []int{},
		indexWeight:      []int{},
		indexAlive:       []bool{},
		hosts:            []string{},
		checks:           []*ProxyCheck{},
	}
	for i, host := range service.Hosts {
		ret.hosts = append(ret.hosts, host.Host)
		ret.indexWeight = append(ret.indexWeight, host.Weight)
		for j := 0; j < host.Weight; j++ {
			ret.requestSequences = append(ret.requestSequences, i)
		}
		ret.indexAlive = append(ret.indexAlive, true)

		if host.Checks != nil {
			for _, check := range host.Checks {
				ret.checks = append(ret.checks, NewProxyCheck(i, host.Host, check))
			}
		}
	}
	rand.Shuffle(len(ret.requestSequences), func(i, j int) {
		ret.requestSequences[i], ret.requestSequences[j] = ret.requestSequences[j], ret.requestSequences[i]
	})
	if service.Checks != nil {
		for _, check := range service.Checks {
			for i, host := range ret.hosts {
				ret.checks = append(ret.checks, NewProxyCheck(i, host, check))
			}
		}
	}
	ret.startHealthCheck()
	return ret
}

func (this *ProxyService) balanceHost() (string, bool) {
	this.mux.Lock()
	defer this.mux.Unlock()

	index, ok := this.balanceIndex()
	debug("using balance index", index, ok)
	if !ok {
		return "", false
	}
	return this.hosts[index], true
}

func (this *ProxyService) balanceIndex() (int, bool) {
	hasAlive := false
	for _, alive := range this.indexAlive {
		if alive {
			hasAlive = true
		}
	}
	if !hasAlive {
		return -1, false
	}
	index := this.requestSequences[this.requestTimes]
	this.requestTimes = (this.requestTimes + 1) % len(this.requestSequences)
	if !this.indexAlive[index] {
		return this.balanceIndex()
	}
	return index, true
}

func (this *ProxyService) startHealthCheck() {
	for _, check := range this.checks {
		go check.run(func(index int) {
			this.mux.Lock()
			defer this.mux.Unlock()
			this.indexAlive[index] = false
		}, func(index int) {
			this.mux.Lock()
			defer this.mux.Unlock()
			this.indexAlive[index] = true
		})
		go check.next()
	}
}

func (this *ProxyService) stopHealthCheck() {
	for _, check := range this.checks {
		check.stop()
	}
}

type ProxyCheck struct {
	index    int
	host     string
	schema   string
	path     string
	method   string
	interval int
	timeout  int
	status   int
	body     string
	window   int
	down     int
	up       int

	nextSignal chan bool
	stopSignal chan bool
	isRunning  bool
	checkPoint []bool
	checkIndex int
	timer      *time.Timer

	isDown bool
	req    *http.Request
	client *http.Client
}

func NewProxyCheck(index int, host string, check *Check) *ProxyCheck {
	ret := &ProxyCheck{
		index:    index,
		host:     host,
		schema:   check.Schema,
		path:     check.Path,
		method:   check.Method,
		interval: check.Interval,
		timeout:  check.Timeout,
		status:   check.Status,
		body:     check.Body,
		window:   check.Window,
		down:     check.Down,
		up:       check.Up,

		nextSignal: make(chan bool),
		stopSignal: make(chan bool),
		checkPoint: []bool{},
		checkIndex: 0,
	}
	if ret.schema == "" {
		ret.schema = "http"
		debug("set check schema default http", index, host)
	}
	if ret.method == "" {
		ret.method = "GET"
		debug("set check method default GET", index, host)
	}
	if ret.window <= 0 {
		ret.window = 10
		debug("set check window default 10", index, host)
	}
	if ret.interval <= 0 {
		ret.interval = 60
		debug("set check interval default 60s", index, host)
	}
	if ret.timeout <= 0 {
		ret.timeout = 5
		debug("set check timeout default 5s", index, host)
	}
	ret.client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:       1,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: true,
		},
		Timeout: time.Duration(ret.timeout) * time.Second,
	}
	if req, err := http.NewRequest(ret.method, ret.schema+"://"+ret.host+ret.path, nil); err != nil {
		debug("parse check url error", ret.host, ret.path, err)
	} else {
		ret.req = req
	}
	return ret
}

func (this *ProxyCheck) run(down func(int), up func(int)) {
	for {
		select {
		case <-this.nextSignal:
			go func() {
				ret := this.check()
				if len(this.checkPoint) < this.window {
					this.checkPoint = append(this.checkPoint, ret)
				} else {
					this.checkPoint[this.checkIndex] = ret
				}
				this.checkIndex = (this.checkIndex + 1) % this.window
				success := 0
				for _, r := range this.checkPoint {
					if r {
						success++
					}
				}
				if !this.isDown && len(this.checkPoint)-success >= this.down {
					debug(fmt.Sprintf(
						"health check down the host at %s, %d(%d) > %d",
						time.Now().Format("2006-01-02 15:04:05"),
						this.window-success,
						this.window,
						this.down,
					))
					down(this.index)
					this.isDown = true
				}
				if this.isDown && success >= this.up {
					debug(fmt.Sprintf(
						"health check up the host at %s, %d(%d) > %d",
						time.Now().Format("2006-01-02 15:04:05"),
						success,
						this.window,
						this.up,
					))
					up(this.index)
					this.isDown = false
				}
				this.next()
			}()
		case <-this.stopSignal:
			return
		}
	}
}

func (this *ProxyCheck) stop() {
	if this.timer != nil {
		this.timer.Stop()
	}
	if this.isRunning {
		this.isRunning = false
		this.stopSignal <- true
	}
}

func (this *ProxyCheck) next() {
	this.isRunning = true
	this.timer = time.AfterFunc(time.Duration(this.interval)*time.Second, func() {
		this.nextSignal <- true
	})
}

func (this *ProxyCheck) check() bool {
	if this.req == nil {
		debug("check failed cause of the bad request", this.method, this.host, this.path)
		return false
	}
	resp, err := this.client.Do(this.req)
	if err != nil {
		debug("check failed cause of request error", err)
		return false
	}
	defer resp.Body.Close()
	if this.status != 0 && resp.StatusCode != this.status {
		debug("check failed cause of unexpect status", this.status, resp.Status)
		return false
	}
	if this.body != "" {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			debug("check failed cause of read response body error", err)
			return false
		}
		if !strings.Contains(string(body), this.body) {
			debug("check failed cause of unexpect body", this.body, string(body))
			return false
		}
	}
	debug("check success", this.req.URL)
	return true
}
