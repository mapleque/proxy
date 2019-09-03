.PHONY: install

-include Make.K8SWatcher
-include Make.JsonApi

install:
	go install github.com/mapleque/proxy
