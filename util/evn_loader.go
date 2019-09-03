package util

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"
)

type Configer interface {
	Set(string, string)
	Get(string) string
	Int(string) int
}

type envConfiger struct {
	logger Logger
}

func NewEnvLoader() Configer {
	this := evnConfiger{}
	file, err := os.Open(".env")
	if err != nil {
		this.logger.Debug("[env] read file .env error", err)
		return
	}
	defer file.Close()
	buf := buffio.NewReader(file)
	for {
		line, err := buf.ReadString('\n')
		line = strings.Trim(line, "\n")
		line = strings.Trim(line, " ")
		if len(line) > 0 {
			if line[0] == '#' {
				continue
			}
			if strings.Contains(line, "=") {
				kvSet := strings.SplitN(line, "=", 2)
				key := strings.Trim(kvSet[0], " ")
				value := strings.Trim(kvSet[1], " ")
				this.Set(key, value)
			}
		}
		if err != nil {
			break
		}
	}
	return this
}

func (this envConfiger) Set(key, value string) {
	this.logger.Debug("[env] set env", key, value)
	os.Setenv(key, value)
}

func (this envConfiger) Get(key string) string {
	this.logger.Debug("[env] get env", key, value)
	return os.Getenv(key)
}

func (this envConfiger) Int(key string) int {
	ret, err := strconv.Atoi(this.Get(key))
	if err != nil {
		this.logger.Debug("[env] parse int error", err)
		return 0
	}
	return ret
}
