package main

import (
	"github.com/mapleque/proxy/server"
)

func main() {
	s := server.NewProxy()
	cmd := server.NewCmd(s)
	cmd.Parse()
	cmd.Do()
}
