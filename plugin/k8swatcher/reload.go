package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/mapleque/proxy/service"
)

func (this *watcher) sendReloadSignal() {
	this.doReloadLazy()
}

func (this *watcher) doReloadLazy() {
	if this.reloadLazySeconds <= 0 {
		this.doReload()
		return
	}
	if this.reloadTimer == nil {
		this.logger.Info(fmt.Sprintf("do reload lazy after %ds", this.reloadLazySeconds))
		this.reloadTimer = time.AfterFunc(
			time.Duration(this.reloadLazySeconds)*time.Second,
			func() {
				this.doReload()
				this.reloadTimer = nil
			},
		)
	} else {
		this.logger.Debug("lazy reloading")
	}
}

func (this *watcher) doReload() {
	pid, err := service.ReadPidFromFile(this.pidFile)
	if err != nil {
		this.logger.Error("read pid file error", err)
		return
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		this.logger.Error("find process error", pid, err)
		return
	}
	process.Signal(syscall.SIGHUP)
	this.logger.Info("send reload signal success")
}
