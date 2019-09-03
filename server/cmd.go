package service

import (
	"flag"
	"fmt"
	"os"
)

// Cmd 命令行参数对象
// 主要用于解析命令行命令
// 并根据命令执行对应的动作
type Cmd struct {
	server     *Proxy
	configFile string
	signalStr  string
	pid        int
	pidFile    string
}

// NewCmd 初始化一个命令行参数对象
func NewCmd(server *Proxy) *Cmd {
	return &Cmd{
		server: server,
	}
}

// Parse 解析命令行参数
func (this *Cmd) Parse() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  To start server as a deamon, run\n")
		fmt.Fprintf(os.Stderr, "  \t`%s` or `%s -c /your/config/file.json`\n", os.Args[0], os.Args[0])
		fmt.Fprintf(os.Stderr, "\nThe args list:\n")
		flag.PrintDefaults()
	}
	flag.StringVar(&this.signalStr, "s", "", `send signal to running server. Avaliable signals:
  stop
  	stop the server
  reload
  	reload all config without stop
`)
	flag.StringVar(&this.configFile, "c", "./config.json", "config file name with absolute path.")
	flag.IntVar(&this.pid, "pid", 0, "the running service pid, only works with signal.")
	flag.StringVar(&this.pidFile, "pid-file", "./proxy.pid", "the running service pid file.")
	help := flag.Bool("h", false, "show this help infomation.")
	flag.BoolVar(&debugEnable, "v", false, "enable debug mode.")

	flag.Parse()
	if *help {
		this.usage()
	}

	if this.configFile == "" {
		this.usage()
	}
}

// Do 根据所解析的参数执行动作
func (this *Cmd) Do() {
	switch this.signalStr {
	case "stop", "reload":
		if this.pid == 0 && this.pidFile != "" {
			if pid, err := ReadPidFromFile(this.pidFile); err != nil {
				this.fatal(err)
			} else {
				this.pid = pid
			}
		}
		switch this.signalStr {
		case "stop":
			if err := this.server.Stop(this.pid); err != nil {
				if errRM := RemoveFile(this.pidFile); errRM != nil {
					this.error(errRM)
				}
				this.fatal(err)
			}
		case "reload":
			if err := this.server.Reload(this.pid); err != nil {
				this.fatal(err)
			}
		}
	default:
		if this.pidFile != "" {
			if CheckFileExist(this.pidFile) {
				if pid, err := ReadPidFromFile(this.pidFile); err != nil {
					if err := RemoveFile(this.pidFile); err != nil {
						this.error(err)
					}
				} else {
					if _, err := os.FindProcess(pid); err != nil {
						debug("found pid file and check it is not running, remove the file")
						if err := RemoveFile(this.pidFile); err != nil {
							this.error(err)
						}
					} else {
						this.fatal("The service has been running. Using `proxy -s` to send a signal and `proxy -h` for more infomation.")
					}
				}
			}
			WritePidToFile(this.pidFile)
			defer func() {
				if err := RemoveFile(this.pidFile); err != nil {
					this.error(err)
				}
			}()
		}
		err := this.server.Start(this.configFile)
		if err != nil {
			this.error(err)
		}
	}
}

func (this *Cmd) fatal(msg interface{}) {
	fmt.Printf("[FATAL]: %v\n", msg)
	os.Exit(1)
}

func (this *Cmd) error(msg interface{}) {
	fmt.Printf("[ERROR]: %v\n", msg)
}

func (this *Cmd) usage() {
	flag.Usage()
	os.Exit(0)
}
