package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type xiaozhiOTARequest struct {
	Application struct {
		Version string `json:"version"`
	} `json:"application"`
}

type xiaozhiOTAServerTime struct {
	Timestamp      int64 `json:"timestamp"`
	TimezoneOffset int   `json:"timezone_offset"`
}

type xiaozhiOTAFirmware struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type xiaozhiOTAWebSocket struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type xiaozhiOTAResponse struct {
	ServerTime xiaozhiOTAServerTime `json:"server_time"`
	Firmware   xiaozhiOTAFirmware   `json:"firmware"`
	WebSocket  xiaozhiOTAWebSocket  `json:"websocket"`
}

type xiaozhiOTAHandler struct {
	profile XiaozhiCompatProfile
}

func NewXiaozhiOTAHandler(profile XiaozhiCompatProfile) http.Handler {
	return xiaozhiOTAHandler{profile: profile}
}

func (h xiaozhiOTAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.addCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("OTA接口运行正常，向设备发送的websocket地址是：" + websocketURLForRequest(r, h.profile.WSPath)))
	case http.MethodPost:
		var req xiaozhiOTARequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_, offsetSeconds := time.Now().Zone()
		writeJSON(w, http.StatusOK, xiaozhiOTAResponse{
			ServerTime: xiaozhiOTAServerTime{
				Timestamp:      time.Now().UnixMilli(),
				TimezoneOffset: offsetSeconds / 60,
			},
			Firmware: xiaozhiOTAFirmware{
				Version: strings.TrimSpace(req.Application.Version),
				URL:     "",
			},
			WebSocket: xiaozhiOTAWebSocket{
				URL:   websocketURLForRequest(r, h.profile.WSPath),
				Token: "",
			},
		})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h xiaozhiOTAHandler) addCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Device-Id, Client-Id, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func websocketURLForRequest(r *http.Request, wsPath string) string {
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	wsScheme := "ws"
	if proto == "https" {
		wsScheme = "wss"
	}
	return wsScheme + "://" + host + wsPath
}
