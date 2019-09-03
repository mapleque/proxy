package util

import (
	"log"
)

type Logger interface {
	Debug(...interface{})
	Info(...interface{})
	Warn(...interface{})
	Error(...interface{})

	SetOutput(string)
	SetDebug(bool)
}

type logger struct {
	debugFlag bool

	debug log.Logger
	info  log.Logger
	warn  log.Logger
	err   log.Logger
}

func NewLogger() Logger {
	this := logger{
		debug: log.New(os.Stdout, "[DEBUG]", log.Ldate|log.Ltime|log.Llongfile),
		info:  log.New(os.Stdout, "[INFO]", log.Ldate|log.Ltime|log.Llongfile),
		warn:  log.New(os.Stdout, "[WARN]", log.Ldate|log.Ltime|log.Llongfile),
		err:   log.New(os.Stdout, "[ERROR]", log.Ldate|log.Ltime|log.Llongfile),
	}
	return this
}

func (this logger) SetOutput(path string) {
	if len(path) > 0 {
		w, err := os.Open(path)
		if err != nil {
			this.Debug("[log] open file error", err)
			return
		}
		this.debug.SetOutput(w)
		this.info.SetOutput(w)
		this.warn.SetOutput(w)
		this.err.SetOutput(w)
	}
}

func (this logger) SetDebug(enable bool) {
	this.debugFlag = enable
}

func (this logger) Debug(msg ...interface{}) {
	if this.debugFlag {
		this.debug.Print(msg...)
	}
}

func (this logger) Info(msg ...interface{}) {
	this.info.Print(msg...)
}

func (this logger) Warn(msg ...interface{}) {
	this.warn.Print(msg...)
}

func (this logger) Error(msg ...interface{}) {
	this.err.Print(msg...)
}
