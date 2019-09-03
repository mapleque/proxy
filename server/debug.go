package service

import (
	"encoding/json"
	"fmt"
)

var (
	debugEnable  = false
	debugDisable = false
)

func debug(msg ...interface{}) {
	if debugEnable && !debugDisable {
		fmt.Print("[DEBUG] ")
		fmt.Println(msg...)
	}
}

func stringify(tar interface{}) string {
	bt, _ := json.MarshalIndent(tar, "", "\t")
	return string(bt)
}
