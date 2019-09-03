package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

type ValidFunc func(raw []byte) bool

var (
	funcMap map[string]ValidFunc
)

func init() {
	funcMap = make(map[string]ValidFunc)
}

func validJson(parent string, fieldName string, fieldType reflect.Type, raw []byte, rule string) error {
	displayName := fieldName
	if parent != "" {
		displayName = fmt.Sprintf("%s.%s", parent, fieldName)
	}
	vr := newValidRule(displayName, fieldType, rule, raw)

	if raw == nil {
		if !vr.optional {
			return vr.requiredError
		}
		return nil
	}

	switch fieldType.Kind() {
	case reflect.Struct:
		if bytes.Equal(raw, []byte("null")) && !strings.Contains(rule, "notnull") {
			return nil
		}
		dst := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &dst); err != nil {
			return vr.typeError
		}
		for i := 0; i < fieldType.NumField(); i++ {
			structField := fieldType.Field(i)
			tarFieldType := structField.Type
			tarFieldName := structField.Name
			if jsonTag, exist := structField.Tag.Lookup("json"); exist {
				tarFieldName = strings.Split(jsonTag, ",")[0]
			}
			rule, exist := structField.Tag.Lookup("valid")
			if !exist {
				rule = "optional"
			}
			var tarRaw []byte
			if tarRawJson, ok := dst[tarFieldName]; ok {
				tarRaw, _ = tarRawJson.MarshalJSON()
			}
			if err := validJson(displayName, tarFieldName, tarFieldType, tarRaw, rule); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		dst := []json.RawMessage{}
		if err := json.Unmarshal(raw, &dst); err != nil {
			return vr.typeError
		}
		tarFieldType := fieldType.Elem()
		for i, tarRawJson := range dst {
			tarFieldName := fmt.Sprintf("%s_%d", fieldName, i)
			tarRaw, _ := tarRawJson.MarshalJSON()
			if err := validJson(displayName, tarFieldName, tarFieldType, tarRaw, rule); err != nil {
				return err
			}
		}
	case reflect.Ptr:
		tarFieldType := fieldType.Elem()
		if err := validJson(parent, fieldName, tarFieldType, raw, rule); err != nil {
			return err
		}
	default:
		if !vr.test(raw) {
			return vr.testError
		}
	}
	return nil
}

type validRule struct {
	raw       string
	fieldType reflect.Type

	optional bool
	rules    []ruler

	requiredError error
	typeError     error
	testError     error
}

const (
	_REG_MODE          = "/.*/"
	_REG_MODE_EXEC     = "/(.*)/"
	_RANGE_MODE        = `[\[\(]-{0,1}\d*,-{0,1}\d*[\]\)]`
	_RANGE_MODE_EXEC   = `([\[\(])(-{0,1}\d*),(-{0,1}\d*)([\]\)])`
	_ENUM_MODE         = `\{[a-zA-Z_\d\.]+(,[a-zA-Z_\d\.]+)*?\}`
	_FUNC_MODE         = "@[a-zA-Z_]+[0-9a-zA-Z_]*"
	_OPTIONAL_MODE     = "optional"
	_MESSAGE_MODE      = `message.*=.*`
	_MESSAGE_MODE_EXEC = `(message.*)=(.*)`
)

func newValidRule(displayName string, fieldType reflect.Type, raw string, value []byte) *validRule {
	ret := &validRule{
		raw:       raw,
		fieldType: fieldType,
		rules:     []ruler{},
	}
	if raw == "" {
		return ret
	}

	matchArr := []string{}
	for _, mode := range []string{
		_OPTIONAL_MODE,
		_MESSAGE_MODE,
		_REG_MODE,
		_RANGE_MODE,
		_FUNC_MODE,
	} {
		if m := regexp.MustCompile("("+mode+")").FindAllStringSubmatch(raw, -1); m != nil && len(m) > 0 {
			for _, tm := range m {
				matchArr = append(matchArr, tm[1])
			}
		}
	}

	debugDisable = true
	for _, rule := range matchArr {
		switch {
		case rule == "":
			continue
		case regexp.MustCompile(_OPTIONAL_MODE).MatchString(rule):
			ret.optional = true
		case regexp.MustCompile(_MESSAGE_MODE).MatchString(rule):
			m := regexp.MustCompile(_MESSAGE_MODE_EXEC).FindStringSubmatch(rule)
			msg := NewVariableExpr(m[2])
			params := NewProxyVariable()
			params.Set("name", displayName)
			params.Set("value", string(value))
			switch m[1] {
			case "message_required":
				ret.requiredError = errors.New(msg.Load(params))
			case "message_type":
				ret.typeError = errors.New(msg.Load(params))
			case "message_test":
				ret.testError = errors.New(msg.Load(params))
			case "message":
				ret.requiredError = errors.New(msg.Load(params))
				ret.typeError = errors.New(msg.Load(params))
				ret.testError = errors.New(msg.Load(params))
			}
		case regexp.MustCompile(_REG_MODE).MatchString(rule):
			ret.rules = append(ret.rules, newRegRule(
				regexp.MustCompile(_REG_MODE_EXEC).FindStringSubmatch(rule)[1],
			))
		case regexp.MustCompile(_RANGE_MODE).MatchString(rule):
			ele := regexp.MustCompile(_RANGE_MODE_EXEC).FindStringSubmatch(rule)
			ret.rules = append(ret.rules, newRangeRule(fieldType, ele[2], ele[3], ele[1] == "[", ele[4] == "]"))
		case regexp.MustCompile(_ENUM_MODE).MatchString(rule):
			ret.rules = append(ret.rules, newEnumRule(rule[1:len(rule)-1]))
		case regexp.MustCompile(_FUNC_MODE).MatchString(rule):
			ret.rules = append(ret.rules, newFuncRule(rule[1:]))
		default:
			panic("regexp not match " + rule)
		}
	}
	debugDisable = false

	if ret.requiredError == nil {
		ret.requiredError = fmt.Errorf("%s：当前字段不能为空", displayName)
	}
	if ret.typeError == nil {
		ret.typeError = fmt.Errorf("%s：当前字段类型不正确", displayName)
	}
	if ret.testError == nil {
		ret.testError = fmt.Errorf("%s：当前字段校验不通过", displayName)
	}
	return ret
}

type ruler interface {
	test(raw []byte) bool
}

func (this *validRule) test(raw []byte) bool {
	for _, r := range this.rules {
		if !r.test(raw) {
			return false
		}
	}
	return true
}

type regRule struct{ reg string }

func newRegRule(reg string) *regRule { return &regRule{reg: reg} }
func (this *regRule) test(raw []byte) bool {
	str := string(raw)
	val := len(str)
	// 如果是字符串，去掉两端的引号
	if val > 2 && str[0] == '"' && str[val-1] == '"' {
		str = str[1 : val-1]
	}
	return regexp.MustCompile(this.reg).MatchString(str)
}

type enumRule struct{ enum []string }

func newEnumRule(rule string) *enumRule { return &enumRule{enum: strings.Split(rule, ",")} }

func (this *enumRule) test(raw []byte) bool {
	str := string(raw)
	val := len(str)
	// 如果是字符串，去掉两端的引号
	if val > 2 && str[0] == '"' && str[val-1] == '"' {
		str = str[1 : val-1]
	}
	for _, e := range this.enum {
		if strings.EqualFold(e, str) {
			return true
		}
	}
	return false
}

type rangeRule struct {
	fieldType reflect.Type
	min       string
	max       string
	equalMin  bool
	equalMax  bool
}

func newRangeRule(fieldType reflect.Type, min, max string, equalMin, equalMax bool) *rangeRule {
	return &rangeRule{
		fieldType: fieldType,
		min:       min,
		max:       max,
		equalMin:  equalMin,
		equalMax:  equalMax,
	}
}

func (this *rangeRule) test(raw []byte) bool {
	switch this.fieldType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int32, reflect.Int64:
		// 如果是int，就比较int值大小
		val, err := strconv.ParseInt(string(raw), 10, 64)
		if err != nil {
			return false
		}
		switch {
		case this.min == "" && this.max == "":
			return true
		case this.min == "":
			max, _ := strconv.ParseInt(this.max, 10, 64)
			return val < max || (val == max && this.equalMax)
		case this.max == "":
			min, _ := strconv.ParseInt(this.min, 10, 64)
			return val > min || (val == min && this.equalMin)
		default:
			min, _ := strconv.ParseInt(this.min, 10, 64)
			max, _ := strconv.ParseInt(this.max, 10, 64)
			return (val > min || (val == min && this.equalMin)) &&
				(val < max || (val == max && this.equalMax))
		}
	case reflect.Float32, reflect.Float64:
		// 如果是float，就比较float值大小
		val, err := strconv.ParseFloat(string(raw), 64)
		if err != nil {
			return false
		}
		switch {
		case this.min == "" && this.max == "":
			return true
		case this.min == "":
			max, _ := strconv.ParseFloat(this.max, 64)
			return val < max || (val == max && this.equalMax)
		case this.max == "":
			min, _ := strconv.ParseFloat(this.min, 64)
			return val > min || (val == min && this.equalMin)
		default:
			min, _ := strconv.ParseFloat(this.min, 64)
			max, _ := strconv.ParseFloat(this.max, 64)
			return (val > min || (val == min && this.equalMin)) &&
				(val < max || (val == max && this.equalMax))
		}
	case reflect.String:
		// 如果是string，就比较长度
		str := string(raw)
		val := len(str)
		// 目标类型不对
		if str[0] != '"' || str[val-1] != '"' {
			return false
		}
		// 去掉两端的引号
		val = val - 2
		switch {
		case this.min == "" && this.max == "":
			return true
		case this.min == "":
			max, _ := strconv.Atoi(this.max)
			return val < max || (val == max && this.equalMax)
		case this.max == "":
			min, _ := strconv.Atoi(this.min)
			return val > min || (val == min && this.equalMin)
		default:
			max, _ := strconv.Atoi(this.max)
			min, _ := strconv.Atoi(this.min)
			return (val > min || (val == min && this.equalMin)) &&
				(val < max || (val == max && this.equalMax))
		}
	default:
		// 如果是其他的，就都算不通过
		return false
	}
}

type funcRule struct{ funcName string }

func newFuncRule(funcName string) *funcRule { return &funcRule{funcName: funcName} }

func (this *funcRule) test(raw []byte) bool {
	if f, exist := funcMap[this.funcName]; !exist {
		return false
	} else {
		return f(raw)
	}
}
