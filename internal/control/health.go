package control

import (
	"net/http"
	"time"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Time    string `json:"time"`
}

func NewHealthHandler(serviceName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: serviceName,
			Time:    time.Now().UTC().Format(time.RFC3339),
		})
	})
}
