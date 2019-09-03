package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/mapleque/proxy/service"
)

func (this *watcher) updateConfig(pod *Pod) error {
	if len(pod.ports) == 0 {
		// no need to proxy
		return errors.New("no need to proxy")
	}
	// load app config file
	filename := this.configFilePath + "apps-k8swatcher/" + pod.GetAppconfigFilename()
	file, err := os.OpenFile(
		filename,
		os.O_RDWR|os.O_CREATE,
		0666,
	)
	if err != nil {
		this.logger.Error(err)
		return err
	}
	defer func() {
		file.Close()
		if pod.waitforDel {
			this.logger.Debug("remove pod file", filename)
			if err := os.Remove(filename); err != nil {
				this.logger.Error("remove app config error", err)
			}
		}
	}()
	body, err := ioutil.ReadAll(file)
	if err != nil {
		this.logger.Error(err)
		return err
	}
	app := &service.App{}
	if len(body) > 0 {
		err := json.Unmarshal(body, app)
		if err != nil {
			this.logger.Error(err, (body))
			return err
		}
	}

	switch pod.op {
	case opTypeAdd:
		if app.Port == 0 {
			this.initAppWithPod(app, pod)
		}
		this.updateApp(app, pod)
		bt, _ := json.MarshalIndent(app, "", "  ")
		file.Truncate(0)
		file.Seek(0, 0)
		file.Write(bt)
		return nil
	case opTypeDel:
		for p, v := range pod.ports {
			this.delPodRule(app, pod, p, v)
			for _, r := range app.Rules {
				this.logger.Debug("pod rule now", r.To)
			}
			this.delPodService(app, pod, p, v)
			for _, s := range app.Services {
				for _, h := range s.Hosts {
					this.logger.Debug("pod service host now", h.Host)
				}
			}
		}
		this.delPod(app, pod)
		bt, _ := json.MarshalIndent(app, "", "  ")
		file.Truncate(0)
		file.Seek(0, 0)
		file.Write(bt)
		return nil
	}
	this.logger.Error("do nothing with unknown op")
	return nil
}

func (this *watcher) initAppWithPod(app *service.App, pod *Pod) {
	app.Port = this.appListenPort
	app.Rules = []*service.Rule{}
	app.Services = []*service.Service{}
	app.AccessLog = &service.Log{
		File:         this.appLogPath + pod.GetName() + ".access.log",
		Fmt:          "access",
		RotateTime:   "day",
		RotateNumber: 7,
	}
	app.ErrorLog = &service.Log{
		File:         this.appLogPath + pod.GetName() + ".error.log",
		Fmt:          "error",
		RotateTime:   "day",
		RotateNumber: 7,
	}
}

func (this *watcher) updateApp(app *service.App, pod *Pod) {
	for p, v := range pod.ports {
		this.addBalanceRule(app, pod, p)
		this.addPodRule(app, pod, p, v)
		this.addPodService(app, pod, p, v)
	}
}

func (this *watcher) addBalanceRule(app *service.App, pod *Pod, p string) {
	domain := pod.GetDomainOnPort(p)
	sdomain := pod.GetDomainOnPort("")
	serviceName := pod.GetServiceNameOnPort(p)

	to := fmt.Sprintf("http://%s/$1", serviceName)
	// check exist
	this.checkAndDo(
		app.Rules,
		func(t interface{}) bool {
			return to == t.(*service.Rule).To
		},
		func(i int) {
			app.Rules = append(app.Rules[:i], app.Rules[i+1:]...)
		},
	)

	// add
	app.Rules = append(app.Rules, &service.Rule{
		Filters: []*service.Filter{
			&service.Filter{Urls: []string{fmt.Sprintf("//%s(?:\\:%d)?/(.*)?", domain, this.appListenPort)}},
			&service.Filter{Urls: []string{fmt.Sprintf("//%s(?:\\:%d)?/(.*)?", sdomain, this.appListenPort)}},
		},
		To: to,
		Transform: &service.Transform{
			Headers: []*service.HeaderTransform{
				&service.HeaderTransform{
					When:   "response",
					Method: "set",
					Key:    "Real-Host",
					Value:  "$real_host",
				},
			},
		},
	})
}

func (this *watcher) addPodRule(app *service.App, pod *Pod, p, v string) {
	domain := pod.GetDomainOnPort(p)
	sdomain := pod.GetDomainOnPort("")
	podHost := pod.GetPodHostOnPort(v)

	to := fmt.Sprintf("http://%s/$1", podHost)
	// check exist
	this.checkAndDo(
		app.Rules,
		func(t interface{}) bool {
			return to == t.(*service.Rule).To
		},
		func(i int) {
			app.Rules = append(app.Rules[:i], app.Rules[i+1:]...)
		},
	)

	// add
	app.Rules = append([]*service.Rule{&service.Rule{
		Filters: []*service.Filter{
			&service.Filter{
				Urls: []string{fmt.Sprintf("//%s(?:\\:%d)?/(.*)?", domain, this.appListenPort)},
				Headers: []*service.HeaderFilter{
					&service.HeaderFilter{
						Key:   "Real-Host",
						Value: podHost,
					},
				},
			},
			&service.Filter{
				Urls: []string{fmt.Sprintf("//%s(?:\\:%d)?/(.*)?", sdomain, this.appListenPort)},
				Headers: []*service.HeaderFilter{
					&service.HeaderFilter{
						Key:   "Real-Host",
						Value: podHost,
					},
				},
			},
		},
		To: to,
	}}, app.Rules...)
}

func (this *watcher) addPodService(app *service.App, pod *Pod, p, v string) {
	serviceName := pod.GetServiceNameOnPort(p)
	podHost := pod.GetPodHostOnPort(v)

	var serv *service.Service
	// check serv exist
	if exist := this.checkAndDo(app.Services, func(t interface{}) bool {
		return serviceName == t.(*service.Service).Name
	}, func(i int) {
		serv = app.Services[i]
	}); !exist {
		// add service
		serv = &service.Service{
			Name:   serviceName,
			Hosts:  []*service.Host{},
			Checks: []*service.Check{},
		}
		app.Services = append(app.Services, serv)
	}
	this.addServicesChecks(serv, pod)

	// check host exist
	this.checkAndDo(
		serv.Hosts,
		func(t interface{}) bool {
			return podHost == t.(*service.Host).Host
		},
		func(i int) {
			serv.Hosts = append(serv.Hosts[:i], serv.Hosts[i+1:]...)
		},
	)
	// add host
	host := &service.Host{
		Host:   podHost,
		Weight: 1,
		Checks: []*service.Check{},
	}
	serv.Hosts = append(serv.Hosts, host)
	this.addHostChecks(host, pod)
}

func (this *watcher) addServicesChecks(serv *service.Service, pod *Pod) {}
func (this *watcher) addHostChecks(host *service.Host, pod *Pod)        {}

func (this *watcher) checkAndDo(arr interface{}, checkFunc func(t interface{}) bool, doFunc func(int)) bool {
	v := reflect.ValueOf(arr)
	index := -1
	for i := 0; i < v.Len(); i++ {
		if checkFunc(v.Index(i).Interface()) {
			index = i
		}
	}
	if index >= 0 && doFunc != nil {
		doFunc(index)
	}
	return index >= 0
}

func (this *watcher) delPodRule(app *service.App, pod *Pod, p, v string) {
	podHost := pod.GetPodHostOnPort(v)
	this.logger.Debug("delete pod host", podHost)
	to := fmt.Sprintf("http://%s/$1", podHost)
	// check exist
	this.checkAndDo(
		app.Rules,
		func(t interface{}) bool {
			return to == t.(*service.Rule).To
		},
		func(i int) {
			app.Rules = append(app.Rules[:i], app.Rules[i+1:]...)
		},
	)
}

func (this *watcher) delPodService(app *service.App, pod *Pod, p, v string) {
	serviceName := pod.GetServiceNameOnPort(p)
	podHost := pod.GetPodHostOnPort(v)
	this.logger.Debug("delete pod service", serviceName, podHost)
	var serv *service.Service
	// check serv exist
	if exist := this.checkAndDo(app.Services, func(t interface{}) bool {
		return serviceName == t.(*service.Service).Name
	}, func(i int) {
		serv = app.Services[i]
	}); !exist {
		this.logger.Error("delete service host failed, cause can not find service", serviceName)
		return
	}
	this.checkAndDo(
		serv.Hosts,
		func(t interface{}) bool {
			return podHost == t.(*service.Host).Host
		},
		func(i int) {
			serv.Hosts = append(serv.Hosts[:i], serv.Hosts[i+1:]...)
		},
	)
	if len(serv.Hosts) == 0 {
		this.checkAndDo(
			app.Services,
			func(t interface{}) bool {
				return serviceName == t.(*service.Service).Name
			},
			func(i int) {
				app.Services = append(app.Services[:i], app.Services[i+1:]...)
			},
		)
	}
}

func (this *watcher) delPod(app *service.App, pod *Pod) {
	if len(app.Services) == 0 {
		pod.waitforDel = true
	}
}
