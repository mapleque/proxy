package main

import (
	"github.com/mapleque/proxy/util"
)

func main() {
	conf := util.NewEnvLoader()
	logger := util.NewLogger()

	logdir := conf.Get("LOG_DIR")
	if logdir != "" {
		logger.SetOutput(logdir + "/k8swatcher.log")
	}

	if conf.Bool("DEBUG") {
		logger.SetDebug(true)
	} else {
		logger.SetDebug(false)
	}

	s, err := newWatcher(
		conf.Get("K8S_WATCH_API"),
		conf.Get("K8S_WATCH_API_TOKEN"),
		conf.Int("K8S_WATCH_RETRY_TIMES"),
		conf.Int("K8S_WATCH_RETRY_SHORT_INTERVAL_SECONDS"),
		conf.Int("K8S_WATCH_RETRY_LONG_INTERVAL_SECONDS"),
		conf.Get("PROXY_CONFIG_FILE_PATH"),
		conf.Get("PROXY_PID_FILE"),
		conf.Int("RELOAD_LAZY_SECONDS"),
		conf.Get("APP_LOG_PATH"),
		conf.Int("APP_LISTEN_PORT"),
		logger,
	)
	if err != nil {
		panic(err)
	}
	s.run()
}
