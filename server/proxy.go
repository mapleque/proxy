package service

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

// Proxy 代理服务对象
// 根据当前配置，启动代理服务
type Proxy struct {
	logger         *ProxyLogger
	config         *Config
	waitingForStop chan interface{}

	servers map[int]*HttpServer
}

// NewServer 新建代理服务对象
func NewProxy() *Proxy {
	return &Proxy{}
}

// Stop 停止正在运行的服务
// 通过发送SIGINT信号，通知服务进程
func (this *Proxy) Stop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	process.Signal(syscall.SIGINT)
	return nil
}

// Reload 重新加载服务配置
// 通过发送SIGHUP信号，通知服务进程
func (this *Proxy) Reload(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	process.Signal(syscall.SIGHUP)
	return nil
}

// Start 启动服务
func (this *Proxy) Start(configFilename string) error {
	this.config = NewConfig()
	if err := this.config.Load(configFilename); err != nil {
		return err
	}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGHUP)
		for s := range sig {
			debug("recieve signal", s)
			switch s {
			case syscall.SIGINT:
				this.stop()
			case syscall.SIGHUP:
				this.reload()
			}
		}
	}()
	if err := this.start(); err != nil {
		return err
	}
	return nil
}

func (this *Proxy) stop() {
	this.logger.Log("recieve stop signal, stoping service ...")
	close(this.waitingForStop)
}

func (this *Proxy) reload() {
	this.logger.Log("recieve reload signal, reloading service ...")
	config := NewConfig()
	if err := config.Load(this.config.entry); err != nil {
		this.logger.Error("reload failed", err)
		return
	}
	this.config = config

	for p, s := range this.servers {
		s.startReload()
		s.setGlobalService(this.config.Services)
		s.setGlobalLogfmt(this.config.Logfmts)
		for _, app := range this.config.Apps {
			if p == app.Port {
				s.appendApp(app)
			}
		}
		s.endReload()
	}
	this.logger.Log("reload success")
}

func (this *Proxy) start() error {
	go func() {
		this.logger.Error(http.ListenAndServe(":9999", nil))
	}()
	this.logger = NewProxyLogger()
	if _, err := this.logger.Load(this.config.Syslog); err != nil {
		return err
	}
	this.waitingForStop = make(chan interface{})
	done := make(chan interface{})
	this.logger.Log("proxy service starting ...")
	go func() {
		defer func() { close(done) }()
		this.servers = this.createHttpServers()
		for _, httpServer := range this.servers {
			defer httpServer.stop()
			go httpServer.run()
		}
		if len(this.servers) == 0 {
			this.logger.Error("no port for listening, stop the service now ...")
			this.stop()
		}
		<-this.waitingForStop
	}()
	<-done
	this.logger.Log("proxy service stoped")
	return nil
}

func (this *Proxy) createHttpServers() map[int]*HttpServer {
	ret := map[int]*HttpServer{}
	for _, app := range this.config.Apps {
		if app == nil {
			continue
		}
		if _, exist := ret[app.Port]; !exist {
			s := &HttpServer{
				logger: this.logger,
			}
			s.setGlobalService(this.config.Services)
			s.setGlobalLogfmt(this.config.Logfmts)
			ret[app.Port] = s
		}
		ret[app.Port].appendApp(app)
	}
	return ret
}
