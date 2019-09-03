package service

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ProxyHandles struct {
	domains  map[string]*ProxyDomain
	services *ProxyServices
	logfmts  *ProxyLogfmts
	logger   *ProxyLogger
}

func NewProxyHandles(logger *ProxyLogger) *ProxyHandles {
	return &ProxyHandles{
		domains: make(map[string]*ProxyDomain),
		logger:  logger,
	}
}

func (this *ProxyHandles) stop() {
	for _, d := range this.domains {
		d.services.stop()
		debug("cleanup hanles services done")
		d.accessLog.Close()
		debug("cleanup hanles access log done")
		d.errorLog.Close()
		debug("cleanup hanles error log done")
	}
}

func (this *ProxyHandles) add(app *App) {
	for _, d := range app.Domains {
		if _, ok := this.domains[d.Domain]; ok {
			this.logger.Error("found duplicate domain:", d.Domain)
		} else {
			this.domains[d.Domain] = this.NewProxyDomain(app, d, this.services, this.logfmts, this.logger)
		}
	}
}

func (this *ProxyHandles) setGlobalService(services *ProxyServices) {
	this.services = services
}

func (this *ProxyHandles) setGlobalLogfmt(logfmts *ProxyLogfmts) {
	this.logfmts = logfmts
}

func (this *ProxyHandles) match(c *Context) (*ProxyHandle, bool) {
	result := strings.Split(c.req.Host, ":")
	var domainString string
	if len(result) == 2 {
		domainString = result[0]
	} else {
		domainString = c.req.Host
	}

	domain, exist := this.domains[domainString]
	debug(fmt.Sprintf("match domain(%v) %s", exist, domainString))
	if !exist {
		return nil, false
	}
	for _, rule := range domain.rules {
		if rule.match(c) {
			return rule, true
		}
	}
	return nil, false
}

type ProxyDomain struct {
	rules     []*ProxyHandle
	services  *ProxyServices
	accessLog *ProxyLogger
	errorLog  *ProxyLogger
	syslog    *ProxyLogger
}

func (this *ProxyHandles) NewProxyDomain(app *App, domain *Domain, services *ProxyServices, logfmts *ProxyLogfmts, syslog *ProxyLogger) *ProxyDomain {
	ret := &ProxyDomain{
		services:  NewProxyServices(app.Services),
		accessLog: NewProxyLogger(),
		errorLog:  NewProxyLogger(),
		syslog:    syslog,
	}

	// 绑定上一级services
	ret.services.addParent(services)

	// init logger
	appLogfmts := NewProxyLogfmts(app.Logfmts)
	if _, err := ret.accessLog.Load(app.AccessLog); err != nil {
		this.logger.Error(err)
	}
	if _, err := ret.accessLog.LoadFmt(appLogfmts); err != nil {
		if logfmts != nil {
			if _, err := ret.accessLog.LoadFmt(logfmts); err != nil {
				this.logger.Error(err)
			}
		}
	}

	if _, err := ret.errorLog.Load(app.ErrorLog); err != nil {
		this.logger.Error(err)
	}
	if _, err := ret.errorLog.LoadFmt(appLogfmts); err != nil {
		if logfmts != nil {
			if _, err := ret.errorLog.LoadFmt(logfmts); err != nil {
				this.logger.Error(err)
			}
		}
	}

	// add rules
	for _, rule := range domain.Rules {
		ret.rules = append(ret.rules, NewProxyHandle(rule, ret.services, ret.accessLog, ret.errorLog, ret.syslog))
	}

	return ret
}

type ProxyHandle struct {
	tr               http.RoundTripper
	filters          []*ProxyHandleFilter
	target           *ProxyTarget
	headerTransforms []*ProxyHeaderTransform
	services         *ProxyServices
	accessLog        *ProxyLogger
	errorLog         *ProxyLogger
	syslog           *ProxyLogger
}

func NewProxyHandle(rule *Rule, services *ProxyServices, accessLogger, errorLogger, sysLogger *ProxyLogger) *ProxyHandle {
	ret := &ProxyHandle{
		tr:               http.DefaultTransport,
		filters:          []*ProxyHandleFilter{},
		target:           NewProxyTarget(rule.To),
		headerTransforms: []*ProxyHeaderTransform{},
		services:         services,
		accessLog:        accessLogger,
		errorLog:         errorLogger,
		syslog:           sysLogger,
	}

	for _, filter := range rule.Filters {
		ret.filters = append(ret.filters, NewProxyHandleFilter(filter))
	}
	if rule.Transform != nil {
		if rule.Transform.Headers != nil {
			for _, headerTransform := range rule.Transform.Headers {
				ret.headerTransforms = append(ret.headerTransforms, NewProxyHeaderTransform(headerTransform))
			}
		}
	}
	return ret
}

func (this *ProxyHandle) match(c *Context) bool {
	if len(this.filters) == 0 {
		return true
	}
	for _, filter := range this.filters {
		if is, err := filter.match(c); err != nil {
			this.syslog.Error(err)
			return false
		} else if is {
			return true
		}
	}
	return false
}

func (this *ProxyHandle) serve(c *Context) {
	hasError := false
	defer func() {
		c.endAt = time.Now()
		c.variables.Set("request_end", c.endAt.Format("2006/01/02 15:04:05"))
		c.variables.Set("latency", fmt.Sprintf("%d", c.endAt.Sub(c.startAt).Nanoseconds()/int64(time.Millisecond)))

		this.accessLog.Logfmt(c.variables)
		if hasError {
			this.errorLog.Logfmt(c.variables)
		}
	}()

	remoteIp := c.req.RemoteAddr
	if xffHost, _, err := net.SplitHostPort(c.req.RemoteAddr); err == nil {
		remoteIp = xffHost
	}
	c.variables.Set("remote_ip", remoteIp)
	for k, _ := range c.req.Header {
		c.variables.Set(fmt.Sprintf("header_%s", k), c.req.Header.Get(k))
	}
	xff := c.req.Header.Get("X-Forward-For")
	if xff == "" {
		xff = remoteIp
	} else {
		xff += ", " + remoteIp
	}
	c.variables.Set("x_forward_for", xff)
	this.servicesBalance(c)

	if err := this.proxyPass(c); err != nil {
		c.variables.Set("error_message", fmt.Sprintf("proxy pass failed %v", err))
		c.variables.Set("status", "500")
		hasError = true
		Handler500(c.w, c.req)
		return
	}
}

func (this *ProxyHandle) servicesBalance(c *Context) {
	this.target.load(c.variables)
	if _, err := this.target.balance(this.services); err != nil {
		c.variables.Set("error_message", fmt.Sprintf("balance failed %v", err))
		this.errorLog.Logfmt(c.variables)
	}
	c.url = this.target.tar
}

func (this *ProxyHandle) transformRequest(req *http.Request, c *Context) {
	for _, transform := range this.headerTransforms {
		if transform.when == "request" {
			transform.processRequest(req, c.variables, this.errorLog)
		}
	}
}

func (this *ProxyHandle) proxyPass(c *Context) error {
	debug("proxy pass to", c.url)
	encodeUrl, err := url.Parse(c.url)
	if err != nil {
		return err
	}
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Host = encodeUrl.Host
			req.URL.Scheme = encodeUrl.Scheme
			req.URL.Path = encodeUrl.Path
			// 改变Host
			req.Host = encodeUrl.Host
			c.variables.Set("real_host", encodeUrl.Host)
			this.transformRequest(req, c)
			debug("proxy request Method:", req.Method, "Url:", req.URL, "Header:", req.Header, "Host:", req.Host)
		},
		FlushInterval: 5 * time.Second,
		ModifyResponse: func(resp *http.Response) error {
			c.variables.Set("status", fmt.Sprintf("%d", resp.StatusCode))
			for k, _ := range resp.Header {
				v := resp.Header.Get(k)
				c.variables.Set(fmt.Sprintf("header_%s", k), v)
			}
			this.transformResponse(resp, c)
			c.resp = resp
			return nil
		},
	}
	rp.ServeHTTP(c.w, c.req)
	return nil
}

func (this *ProxyHandle) transformResponse(resp *http.Response, c *Context) {
	for _, transform := range this.headerTransforms {
		if transform.when == "response" {
			transform.processResponse(resp, c.variables, this.errorLog)
		}
	}
}

type ProxyHandleFilter struct {
	requestURIs []*ProxyHandleFilterRequestURI
	headers     []*ProxyHandleFilterHeader
}

func NewProxyHandleFilter(filter *Filter) *ProxyHandleFilter {
	ret := &ProxyHandleFilter{
		requestURIs: []*ProxyHandleFilterRequestURI{},
		headers:     []*ProxyHandleFilterHeader{},
	}
	for _, uri := range filter.RequestURIs {
		ret.requestURIs = append(ret.requestURIs, NewProxyHandleFilterRequestURI(uri))
	}
	for _, header := range filter.Headers {
		ret.headers = append(ret.headers, NewProxyHandleFilterHeader(header))
	}
	return ret
}

func (this *ProxyHandleFilter) match(c *Context) (bool, error) {
	match := false
	if len(this.requestURIs) == 0 {
		match = true
	}
	c.req.URL.Host = c.req.Host
	for _, uri := range this.requestURIs {
		if is, err := uri.match(c); err != nil {
			return false, err
		} else if is {
			match = true
		}
	}
	if !match {
		return false, nil
	}

	if len(this.headers) == 0 {
		return true, nil
	}

	for _, header := range this.headers {
		if header.match(c) {
			return true, nil
		}
	}
	return false, nil
}

type ProxyHandleFilterRequestURI struct {
	value string
}

func NewProxyHandleFilterRequestURI(value string) *ProxyHandleFilterRequestURI {
	return &ProxyHandleFilterRequestURI{value: value}
}

func (this *ProxyHandleFilterRequestURI) match(c *Context) (bool, error) {
	re, err := regexp.Compile(this.value)
	if err != nil {
		return false, err
	}
	ret := re.MatchString(c.req.URL.RequestURI())
	debug(fmt.Sprintf("match request_uri(%v) %s->%s", ret, this.value, c.req.URL.RequestURI()))
	if ret {
		g := re.FindAllStringSubmatch(c.req.URL.RequestURI(), -1)
		for i, ge := range g[0] {
			c.variables.Set(fmt.Sprintf("%d", i), ge)
		}
	}
	return ret, nil
}

type ProxyHandleFilterHeader struct {
	key   string
	value *VariableExpr
}

func NewProxyHandleFilterHeader(header *HeaderFilter) *ProxyHandleFilterHeader {
	return &ProxyHandleFilterHeader{
		key:   header.Key,
		value: NewVariableExpr(header.Value),
	}
}

func (this *ProxyHandleFilterHeader) match(c *Context) bool {
	ret := strings.EqualFold(this.value.Load(c.variables), c.req.Header.Get(this.key))
	debug(fmt.Sprintf("match header(%v) %s->%s", ret, this.value.Load(c.variables), c.req.Header.Get(this.key)))
	return ret
}

type ProxyTarget struct {
	src string
	tar string
}

func NewProxyTarget(url string) *ProxyTarget {
	return &ProxyTarget{src: url, tar: url}
}

func (this *ProxyTarget) load(variables *ProxyVariable) {
	url := NewVariableExpr(this.src)
	this.tar = url.Load(variables)
	debug("trans url", this.src, this.tar)
}

func (this *ProxyTarget) balance(services *ProxyServices) (bool, error) {
	if services == nil {
		debug("balance services is nil")
		return false, nil
	}
	u, err := url.Parse(this.tar)
	if err != nil {
		debug("balance target url parse error", this.tar)
		return false, err
	}
	if u.Host == "" || strings.ContainsAny(u.Host, ". & :") {
		debug("balance target is a domain", u)
		return true, nil
	}
	if host, ok := services.balanceHost(u.Host); ok {
		u.Host = host
		this.tar = u.String()
		debug("balance success to", this.tar)
		return true, nil
	}
	debug("balance failed", this.tar)
	return false, nil
}

type ProxyHeaderTransform struct {
	when    string
	method  string
	key     string
	value   *VariableExpr
	pattern string
}

func NewProxyHeaderTransform(headerTransform *HeaderTransform) *ProxyHeaderTransform {
	return &ProxyHeaderTransform{
		when:    headerTransform.When,
		method:  headerTransform.Method,
		key:     headerTransform.Key,
		value:   NewVariableExpr(headerTransform.Value),
		pattern: headerTransform.Pattern,
	}
}

func (this *ProxyHeaderTransform) processRequest(r *http.Request, variables *ProxyVariable, errorlog *ProxyLogger) {
	if this.pattern != "" {
		if header := r.Header.Get(this.key); header != "" {
			re, err := regexp.Compile(this.pattern)
			if err != nil {
				errorlog.Error(err)
				return
			}
			ret := re.MatchString(header)
			debug(fmt.Sprintf("match header(%v) %s->%s", ret, this.pattern, header))
			if ret {
				g := re.FindAllStringSubmatch(header, -1)
				for i, ge := range g[0] {
					variables.Set(fmt.Sprintf("%d", i), ge)
				}
			} else {
				return
			}
		}
	}

	switch this.method {
	case "add":
		value := this.value.Load(variables)
		debug("add header in request", this.key, value)
		r.Header.Add(this.key, value)
	case "set":
		value := this.value.Load(variables)
		debug("set header in request", this.key, value)
		r.Header.Set(this.key, value)
	case "del":
		debug("del header in request", this.key)
		r.Header.Del(this.key)
	}
}

func (this *ProxyHeaderTransform) processResponse(r *http.Response, variables *ProxyVariable, errorlog *ProxyLogger) {
	if this.pattern != "" {
		if header := r.Header.Get(this.key); header != "" {
			re, err := regexp.Compile(this.pattern)
			if err != nil {
				errorlog.Error(err)
				return
			}
			ret := re.MatchString(header)
			debug(fmt.Sprintf("match header(%v) %s->%s", ret, this.pattern, header))
			if ret {
				g := re.FindAllStringSubmatch(header, -1)
				for i, ge := range g[0] {
					variables.Set(fmt.Sprintf("%d", i), ge)
				}
			} else {
				return
			}
		}
	}

	switch this.method {
	case "add":
		value := this.value.Load(variables)
		debug("add header in response", this.key, value)
		r.Header.Add(this.key, value)
	case "set":
		value := this.value.Load(variables)
		debug("set header in response", this.key, value)
		r.Header.Set(this.key, value)
	case "del":
		debug("del header in response", this.key)
		r.Header.Del(this.key)
	}
}
