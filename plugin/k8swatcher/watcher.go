package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mapleque/proxy/util"
)

type watcher struct {
	api                string
	token              string
	maxRetryTimes      int
	retryShortInterval int
	retryLongInterval  int
	logger             util.Logger

	configFilePath    string
	pidFile           string
	reloadLazySeconds int

	appLogPath    string
	appListenPort int

	client      *http.Client
	req         *http.Request
	retryTimes  int
	err         error
	resetTimer  *time.Timer
	reloadTimer *time.Timer

	podPool map[string]string
}

func newWatcher(
	api, token string, maxRetryTimes, retryShortInterval, retryLongInterval int,
	configFilePath, pidFile string, reloadLazySeconds int,
	appLogPath string, appListenPort int,
	logger util.Logger,
) (*watcher, error) {
	if api == "" {
		return nil, errors.New("api is empty")
	}
	if maxRetryTimes == 0 {
		return nil, errors.New("max retry times is 0")
	}
	if retryShortInterval == 0 {
		return nil, errors.New("retry shot interval is 0")
	}
	if retryLongInterval == 0 {
		return nil, errors.New("retry long interval is 0")
	}
	if configFilePath == "" {
		return nil, errors.New("config file path is emtpy")
	}
	if pidFile == "" {
		return nil, errors.New("pid file is empty")
	}
	if appLogPath == "" {
		return nil, errors.New("app log path is emtpy")
	}
	if appListenPort == 0 {
		return nil, errors.New("app listen port is 0")
	}
	ret := &watcher{
		api:                api,
		token:              token,
		maxRetryTimes:      maxRetryTimes,
		retryShortInterval: retryShortInterval,
		retryLongInterval:  retryLongInterval,
		configFilePath:     configFilePath,
		pidFile:            pidFile,
		reloadLazySeconds:  reloadLazySeconds,
		appLogPath:         appLogPath,
		appListenPort:      appListenPort,
		logger:             logger,

		podPool: map[string]string{},
	}
	// do not verify tls
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	ret.client = client
	req, err := http.NewRequest("GET", api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", token)
	ret.req = req

	return ret, nil
}

func (this *watcher) run() {
	for {
		if this.retryTimes == 0 {
			// 首次启动，直接开始
		} else if this.retryTimes >= this.maxRetryTimes {
			this.logger.Error(fmt.Sprintf(
				"it has been retry %d times in %ds interval, from now on retry in %ds interval.",
				this.maxRetryTimes,
				this.retryShortInterval,
				this.retryLongInterval,
			))
			this.logger.Warn("reconnect", this.retryTimes, "after", this.retryLongInterval)
			time.Sleep(time.Duration(this.retryLongInterval) * time.Second)
		} else {
			this.logger.Warn("reconnect", this.retryTimes, "after", this.retryShortInterval)
			time.Sleep(time.Duration(this.retryShortInterval) * time.Second)
		}
		if this.retryTimes > 0 {
			// 正常运行指定时间，就重置重试次数
			this.resetTimer = time.AfterFunc(
				2*time.Duration(this.retryShortInterval)*time.Second,
				func() {
					this.logger.Info("reset reconnect times")
					this.retryTimes = 0
					this.resetTimer = nil
				},
			)
		}
		if this.err = this.watchK8S(); this.err != nil {
			this.logger.Warn("watch error", this.err)
			this.retryTimes++
			if this.resetTimer != nil {
				this.resetTimer.Stop()
			}
		}
	}
}

func (this *watcher) watchK8S() error {
	resp, err := this.client.Do(this.req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 先删已经生成的配置文件，再接收watch的数据
	if err := os.RemoveAll(this.configFilePath + "apps-k8swatcher/"); err != nil {
		panic(err)
	}
	if err := os.Mkdir(this.configFilePath+"apps-k8swatcher", 0777); err != nil {
		panic(err)
	}

	obj := bytes.NewBuffer([]byte{})
	sep := []byte("\n")
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		obj.Write(buf[:n])
		if err != nil {
			this.logger.Debug(err, string(obj.Bytes()))
			return err
		}
		for bytes.Contains(obj.Bytes(), sep) {
			body, _ := obj.ReadBytes(sep[0])
			pod, err := this.NewPod(body)
			if err != nil {
				this.logger.Error(err, string(body))
			} else {
				this.logger.Debug(string(body))
				this.logger.Info(pod.String())
				switch pod.op {
				case opTypeAdd, opTypeDel:
					if err := this.updateConfig(pod); err == nil {
						this.sendReloadSignal()
					}
				}
			}
		}
	}
}
