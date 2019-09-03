package service

import (
	"net/http"
)

func Handler404(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.Header().Set("Proxy-Error-Status", "404")
	w.WriteHeader(404)
}

func Handler500(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
	w.Header().Set("Proxy-Error-Status", "500")
	w.WriteHeader(500)
}
