package service

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
)

func ReadPidFromFile(pidfile string) (int, error) {
	file, err := os.Open(pidfile)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	body, err := ioutil.ReadAll(file)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(string(body))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func WritePidToFile(pidfile string) error {
	pid := os.Getpid()
	return ioutil.WriteFile(pidfile, []byte(fmt.Sprintf("%d", pid)), 0644)
}

func CheckFileExist(pidfile string) bool {
	_, err := os.Stat(pidfile)
	return !os.IsNotExist(err)
}

func RemoveFile(pidfile string) error {
	return os.Remove(pidfile)
}
