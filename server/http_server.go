package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type HttpServer struct {
	mux      sync.RWMutex
	logger   *ProxyLogger
	port     int
	certFile string
	keyFile  string
	server   *http.Server

	handles  *ProxyHandles
	services *ProxyServices
	logfmts  *ProxyLogfmts

	reloadHandles  *ProxyHandles
	reloadServices *ProxyServices
}

func (this *HttpServer) run() {
	this.server = &http.Server{
		Addr:           fmt.Sprintf(":%d", this.port),
		Handler:        this,
		MaxHeaderBytes: 0x10000,
	}
	if this.certFile != "" && this.keyFile != "" {
		this.logger.Log(fmt.Sprintf("https server listen on %d with %s and %s", this.port, this.certFile, this.keyFile))
		if err := this.server.ListenAndServeTLS(this.certFile, this.keyFile); err != nil {
			this.logger.Error(err)
		} else {
			this.logger.Error(fmt.Sprintf("https server on %d donw"), this.port)
		}
	} else {
		this.logger.Log(fmt.Sprintf("http server listen on %d", this.port))
		if err := this.server.ListenAndServe(); err != nil {
			this.logger.Error(err)
		} else {
			this.logger.Error(fmt.Sprintf("https server on %d donw"), this.port)
		}
	}
}

func (this *HttpServer) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	done := make(chan interface{})
	go func() {
		defer close(done)
		this.logger.Log(fmt.Sprintf("server on %d start shutdown...", this.port))
		this.server.Shutdown(ctx)
	}()
	select {
	case <-done:
		this.logger.Log(fmt.Sprintf("server on %d shutdown success", this.port))
	case <-ctx.Done():
		this.logger.Log(fmt.Sprintf("server on %d shutdown failed, calling Close for stop", this.port))
		this.server.Close()
	}
}

func (this *HttpServer) startReload() {
	this.mux.Lock()
	debug("get lock, start reload http server", this.port)
	this.reloadHandles = this.handles
	this.handles = nil
	this.reloadServices = this.services
	this.services = nil
	this.logfmts = nil
}

func (this *HttpServer) endReload() {
	debug("server reload done, do cleanup", this.port)
	this.mux.Unlock()
	this.reloadServices.stop()
	this.reloadServices = nil
	debug("cleanup services done", this.port)
	this.reloadHandles.stop()
	this.reloadHandles = nil
	debug("cleanup handles done", this.port)
}

func (this *HttpServer) appendApp(app *App) {
	// 相同的port，要求协议必须相同
	this.port = app.Port
	this.certFile = app.CertFile
	this.keyFile = app.KeyFile

	// 其他属性可以按app独享
	if this.handles == nil {
		this.handles = NewProxyHandles(this.logger)
		this.handles.setGlobalService(this.services)
		this.handles.setGlobalLogfmt(this.logfmts)
	}
	this.handles.add(app)
}

func (this *HttpServer) setGlobalService(services []*Service) {
	this.services = NewProxyServices(services)
}

func (this *HttpServer) setGlobalLogfmt(logfmts []*Logfmt) {
	this.logfmts = NewProxyLogfmts(logfmts)
}

func (this *HttpServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := NewContext(w, req)
	c.startAt = time.Now()
	c.variables.Set("request_start", c.startAt.Format("2006/01/02 15:04:05"))

	c.variables.Set("method", c.req.Method)
	c.variables.Set("host", c.req.Host)
	c.variables.Set("uri_path", c.req.URL.Path)
	c.variables.Set("uri_query", c.req.URL.RawQuery)
	c.variables.Set("request_uri", c.req.RequestURI)

	handle, exist := this.handles.match(c)
	if !exist {
		debug("can not find match handle")
		Handler404(w, req)
		return
	}
	handle.serve(c)
}
