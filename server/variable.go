package service

import (
	"fmt"
	"strings"
	"sync"
)

type VariableExpr struct {
	expr string
}

func NewVariableExpr(expr string) *VariableExpr {
	return &VariableExpr{expr: expr}
}

func (this *VariableExpr) Load(params *ProxyVariable) string {
	params.mux.RLock()
	defer params.mux.RUnlock()
	ret := this.expr
	for k, v := range params.data {
		ret = strings.Replace(ret, fmt.Sprintf("$%s", k), v, -1)
	}
	debug(fmt.Sprintf("load variables '%s'->'%s' with \n%s", this.expr, ret, stringify(params.data)))
	return ret
}

type ProxyVariable struct {
	mux  sync.RWMutex
	data map[string]string
}

func NewProxyVariable() *ProxyVariable {
	return &ProxyVariable{
		data: map[string]string{},
	}
}

func (this *ProxyVariable) Set(key, value string) {
	this.mux.Lock()
	defer this.mux.Unlock()
	this.data[key] = value
}
