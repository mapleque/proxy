package service

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type ProxyLogger struct {
	mux     sync.Mutex
	file    string
	handler *os.File
	logger  *log.Logger

	fmt   string
	lines []string

	rotateTime   string
	rotateSize   int64
	rotateNumber int

	rotateTimer *time.Timer
}

func NewProxyLogger() *ProxyLogger {
	return &ProxyLogger{
		logger: log.New(os.Stdout, "", 0),
		lines:  []string{},
	}
}

func (this *ProxyLogger) Close() {
	if this.handler != nil {
		this.logger = log.New(os.Stdout, "", 0)
		this.handler.Close()
	}
	this.stopRotateTime()
}

func (this *ProxyLogger) Load(config *Log) (*ProxyLogger, error) {
	this.stopRotateTime()
	this.lock()
	defer this.unlock()
	if config == nil {
		config = &Log{
			Fmt: "default",
		}
	}
	if config.File == "" {
		this.file = ""
		this.logger = log.New(os.Stdout, "", 0)
		if this.handler != nil {
			this.handler.Close()
			this.handler = nil
		}
	} else {
		if handler, err := mkdirAndOpenFile(config.File); err != nil {
			return nil, err
		} else {
			this.file = config.File
			this.handler = handler
			this.logger = log.New(this.handler, "", 0)
		}
	}
	this.fmt = config.Fmt
	this.rotateTime = config.RotateTime
	this.rotateSize = config.RotateSize
	this.rotateNumber = config.RotateNumber
	if this.rotateNumber < 1 {
		this.rotateNumber = 1
	}
	if this.rotateTime != "" {
		this.startRotateTime()
	}
	return this, nil
}

func (this *ProxyLogger) LoadFmt(fmts *ProxyLogfmts) (*ProxyLogger, error) {
	this.lock()
	defer this.unlock()
	if lines, exist := fmts.find(this.fmt); !exist {
		if this.fmt == "default" {
			this.lines = []string{"[WARN] default logfmt is not define"}
			return this, fmt.Errorf("can not find log fmt %s\n", this.fmt)
		}
		return nil, fmt.Errorf("can not find log fmt %s\n", this.fmt)
	} else {
		this.lines = lines
	}
	debug("load fmt success", this.fmt)
	return this, nil
}

func (this *ProxyLogger) Log(msg ...interface{}) {
	this.lock()
	defer this.unlock()
	this.logger.Println(append([]interface{}{time.Now().Format("2006-01-02 15:04:05"), "[INFO]"}, msg...)...)
	this.checkRotate()
}

func (this *ProxyLogger) Error(msg ...interface{}) {
	this.lock()
	defer this.unlock()
	this.logger.Println(append([]interface{}{time.Now().Format("2006-01-02 15:04:05"), "[ERROR]"}, msg...)...)
	this.checkRotate()
}

func (this *ProxyLogger) Logfmt(variables *ProxyVariable) {
	this.lock()
	defer this.unlock()
	for _, line := range this.lines {
		this.logger.Println(NewVariableExpr(line).Load(variables))
	}
	this.checkRotate()
}

func (this *ProxyLogger) lock() {
	this.mux.Lock()
}

func (this *ProxyLogger) unlock() {
	this.mux.Unlock()
}

func (this *ProxyLogger) checkRotate() {
	if this.file == "" {
		return
	}

	if this.rotateSize > 0 {
		fileInfo, err := os.Stat(this.file)
		if err != nil {
			debug("file rotate stat error", err)
		}
		if fileInfo.Size() > this.rotateSize {
			this.handler.Close()
			// rotate by size
			// mv all file ext +1
			// create and open new file
			for i := this.rotateNumber; i > 0; i-- {
				from := fmt.Sprintf("%s.%d", this.file, i-1)
				to := fmt.Sprintf("%s.%d", this.file, i)
				if i == 1 {
					from = this.file
				}
				if _, err := os.Stat(to); err == nil || os.IsExist(err) {
					os.Remove(to)
				}
				os.Rename(from, to)
			}
			this.handler, _ = mkdirAndOpenFile(this.file)
			this.logger = log.New(this.handler, "", 0)
		}
	}
}

func (this *ProxyLogger) startRotateTime() {
	this.nextRotateTime()
}

func (this *ProxyLogger) nextRotateTime() {
	if this.rotateTime == "" {
		return
	}
	now := time.Now()
	var d time.Duration
	var to string
	switch this.rotateTime {
	case "hour":
		d = now.Truncate(time.Hour).Add(time.Hour).Sub(now)
		to = fmt.Sprintf("%s.%s", this.file, now.Format("2006010215"))
	case "day":
		d = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local).Sub(now)
		to = fmt.Sprintf("%s.%s", this.file, now.Format("20060102"))
	}
	this.rotateTimer = time.AfterFunc(d+time.Second, func() {
		this.handler.Close()
		if _, err := os.Stat(to); err == nil || os.IsExist(err) {
			os.Remove(to)
		}
		os.Rename(this.file, to)
		this.handler, _ = mkdirAndOpenFile(this.file)
		this.logger = log.New(this.handler, "", 0)
		this.nextRotateTime()
	})
}
func (this *ProxyLogger) stopRotateTime() {
	if this.rotateTimer != nil {
		this.rotateTimer.Stop()
	}
}

type ProxyLogfmts struct {
	m map[string][]string
}

func NewProxyLogfmts(logfmts []*Logfmt) *ProxyLogfmts {
	ret := &ProxyLogfmts{m: map[string][]string{}}
	if logfmts != nil {
		for _, fmt := range logfmts {
			if fmt == nil {
				continue
			}
			ret.m[fmt.Name] = fmt.Lines
		}
	}
	return ret
}

func (this *ProxyLogfmts) find(name string) ([]string, bool) {
	lines, exist := this.m[name]
	return lines, exist
}

func mkdirAndOpenFile(filepath string) (*os.File, error) {
	if sepIndex := strings.LastIndex(filepath, "/"); sepIndex > 0 {
		logdir := filepath[0:sepIndex]
		if err := os.MkdirAll(logdir, 0777); err != nil {
			return nil, err
		}
	}

	if fileHandler, err := os.OpenFile(
		filepath,
		os.O_RDWR|os.O_APPEND|os.O_CREATE,
		0666,
	); err != nil {
		return nil, err
	} else {
		return fileHandler, nil
	}
}
