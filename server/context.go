package service

import (
	"net/http"
	"time"
)

type Context struct {
	req  *http.Request
	resp *http.Response
	w    http.ResponseWriter
	url  string

	startAt   time.Time
	endAt     time.Time
	variables *ProxyVariable
}

func NewContext(w http.ResponseWriter, req *http.Request) *Context {
	// 这里如果改成可复用机制
	// 需要实验验证意义有多大
	return &Context{
		req:       req,
		w:         w,
		variables: NewProxyVariable(),
	}
}
