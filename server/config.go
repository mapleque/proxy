package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var (
	INVALID_CONFIG_FILE_TYPE   = errors.New("The configure file must be a json file with .json ext")
	INVALID_CONFIG_FILE_FORMAT = errors.New("The configure file content has invalid json format")
)

// Config 系统配置对象
type Config struct {
	entry string
	raw   []byte

	Apps     []*App     `json:"apps,omitempty" valid:"message_required=$name是必选项,message_type=$name必须是app数组"`
	Services []*Service `json:"services,omitempty" valid:"optional,message_type=$name必须是service数组"`
	Logfmts  []*Logfmt  `json:"logfmts,omitempty" valid:"optional,message_type=$name必须是logfmt数组"`
	Syslog   *Log       `json:"syslog,omitempty" valid:"optional,message_type=$name非法的log对象"`
}

type App struct {
	Domains   []*Domain  `json:"domains,omitempty" valid:"/^[a-z1-9-_\\.]+$a/,message_required=$name是必选项,message=$name($value)是非法域名"`
	Port      int        `json:"port,omitempty" valid:"[1,65535],message_required=$name是必选项,message=$name($value)是非法端口号"`
	CertFile  string     `json:"certfile,omitempty" valid:"optional,message=$name($value)文件路径不正确"`
	KeyFile   string     `json:"keyfile,omitempty" valid:"optional,message=$name($value)文件路径不正确"`
	Services  []*Service `json:"services,omitempty" valid:"optional,message_type=$name必须是service数组"`
	AccessLog *Log       `json:"access_log,omitempty" valid:"optional,message_type=$name必须是log数组"`
	ErrorLog  *Log       `json:"error_log,omitempty" valid:"optional,message_type=$name必须是log数组"`
	Logfmts   []*Logfmt  `json:"logfmts,omitempty" valid:"optional,message_type=$name必须是logfmt数组"`
}

type Domain struct {
	Domain string  `json:"domain,omitempty" valid:"/^[a-z0-9-_\\.]+$/,message_required=$name是必选项,message=$name($value)是非法域名"`
	Rules  []*Rule `json:"rules,omitempty" valid:"message_required=$name是必选项,message_type=$name必须是rule数组"`
}

type Rule struct {
	Filters   []*Filter  `json:"filters,omitempty" valid:"optional,message_type=$name非法的filter对象"`
	To        string     `json:"to,omitempty" valid:"[1,],message=$name($value)不合法"`
	Transform *Transform `json:"transform,omitempty" valid:"optional,message_type=$name($value)非法的transform对象"`
}

type Filter struct {
	RequestURIs []string        `json:"request_uris,omitempty" valid:"optional,message=$name($value)不合法"`
	Headers     []*HeaderFilter `json:"headers,omitempty" valid:"optional,message=$name非法的header_filter对象"`
}

type HeaderFilter struct {
	Key   string `json:"key,omitempty" valid:"/[A-Za-z0-9_\\-]+/,message=$name非法的Http Header Key"`
	Value string `json:"value,omitempty" valid:"message=$name非法的Http Header Value"`
}

type Transform struct {
	Headers []*HeaderTransform `json:"headers,omitempty" valid:"optional,message_type=$name非法的header_transform对象"`
}

type HeaderTransform struct {
	When    string `json:"when,omitempty" valid:"{request,response},message=$name($value)不合法"`
	Method  string `json:"method,omitempty" valid:"{add,set,get},message=$name($value)不合法"`
	Key     string `json:"key,omitempty" valid:"/[A-Za-z0-9_\\-]+/,message=$name非法的Http Header Key"`
	Value   string `json:"value,omitempty" vaild:"optional,message=$name非法的Http Header Value"`
	Pattern string `json:"pattern,omitempty" vaild:"optional,message=$name非法的pattern"`
}

type Service struct {
	Name   string   `json:"name,omitempty" valid:"/[a-z0-9_\\-]+/,message=$name($value)不合法"`
	Hosts  []*Host  `json:"hosts,omitempty" valid:"message=$name必须是host数组"`
	Checks []*Check `json:"checks,omitempty" valid:"optional,message=$name必须是check数组"`
}

type Host struct {
	Host   string   `json:"host,omitempty" valid:"/[a-zA-Z0-9_\\:\\-]+/,message=$name不合法"`
	Weight int      `json:"weight,omitempty" valid:"[1-100],message=$name请填写1-100的整数"`
	Checks []*Check `json:"checks,omitempty" valid:"optional,message=$name必须是check数组"`
}

type Check struct {
	Schema   string `json:"schema,omitempty" valid:"optional,{http,https},message=$name($value)不合法"`
	Path     string `json:"path,omitempty" valid:"[1,],message=$name($value)不合法"`
	Method   string `json:"method,omitempty" valid:"{POST,GET},message=$name($value)不合法"`
	Interval int    `json:"interval,omitempty" valid:"[1,3600],message=$name($value)不合法"`
	Timeout  int    `json:"timeout,omitempty" valid:"optional,[1,3600],message=$name($value)不合法"`
	Status   int    `json:"status,omitempty" valid:"optional,[100,600],message=$name($value)不合法"`
	Body     string `json:"body,omitempty" valid:"optional,message=$name{$value)不合法"`
	Window   int    `json:"window,omitempty" valid:"[1,3600],message=$name($value)不合法"`
	Down     int    `json:"down,omitempty" valid:"[1,3600],message=$name($value)不合法"`
	Up       int    `json:"up,omitempty" valid:"[1,3600],message=$name($value)不合法"`
}

type Logfmt struct {
	Name  string   `json:"name,omitempty" valid:"/[a-z0-9_]+/,message=$name不合法"`
	Lines []string `json:"lines,omitempty" valid:"message=$name必须是字符串数组"`
}

type Log struct {
	File         string `json:"file,omitempty" valid:"message=$name($value)不合法"`
	Fmt          string `json:"fmt,omitempty" valid:"optional,message=$name($value)不合法"`
	RotateTime   string `json:"rotate_time,omitempty" valid:"optional,{hour|day},message=$name($value)不合法"`
	RotateSize   int64  `json:"rotate_size,omitempty" valid:"optional,(0,),message=$name($value)必须是正整数"`
	RotateNumber int    `json:"rotate_number,omitempty" valid:"optional,(0,),message=$name($value)必须是正整数"`
}

// 初始化一个系统配置
func NewConfig() *Config {
	return &Config{}
}

// Load 从文件中加载配置参数
func (this *Config) Load(filename string) error {
	raw, err := readIncludeFiles(filename)
	if err != nil {
		return err
	}
	debug("load config", string(raw))

	if err := this.test(raw); err != nil {
		return err
	}
	this.entry = filename
	this.raw = raw

	if err := json.Unmarshal(this.raw, this); err != nil {
		fmt.Println(string(this.raw))
		return err
	}
	return nil
}

func (this *Config) test(raw []byte) error {
	return validJson(
		this.entry,
		"config",
		reflect.TypeOf(this),
		raw,
		"message_required=配置文件不能为空,message_type=配置文件格式不正确",
	)

}

// readIncludeFiles 读包含@include语法的文件
// 对于文件中的`@include [a-zA-Z0-9_/\.]+\.json`字符串，使用读取到的文件内容替换
func readIncludeFiles(filename string) ([]byte, error) {
	currentPath := filepath.Dir(filename)
	if strings.Contains(filename, "*") {
		body, err := readFuzzyFiles(filename)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ext := path.Ext(file.Name())
	if ext != ".json" {
		fmt.Println(file.Name())
		return nil, INVALID_CONFIG_FILE_TYPE
	}
	body, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile("\"@include ([a-zA-Z0-9_/\\.\\*\\-]+\\.json)\"")
	var includeErr error
	content := re.ReplaceAllFunc(body, func(src []byte) []byte {
		g := re.FindSubmatch(src)
		includeFile := string(g[1])
		if includeFile[0] != '/' {
			// read relative path file
			includeFile = filepath.Join(currentPath, includeFile)
		}
		raw, err := readIncludeFiles(includeFile)
		if err != nil {
			includeErr = err
			return []byte(err.Error())
		}
		return raw
	})
	if includeErr != nil {
		return nil, includeErr
	}
	return content, nil
}

func readFuzzyFiles(filename string) ([]byte, error) {
	fileList, err := filepath.Glob(filename)
	if err != nil {
		return nil, err
	}
	arr := [][]byte{}
	for _, f := range fileList {
		bts, err := readIncludeFiles(f)
		if err != nil {
			return nil, err
		}
		arr = append(arr, bts)
	}
	body := bytes.Join(arr, []byte(","))
	if len(body) == 0 {
		body = []byte("null")
	}
	return body, nil
}
