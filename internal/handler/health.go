package handler

import (
	"context"
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
}

type Pinger interface {
	Ping(ctx context.Context) error
}

func Liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func ReadinessWithDB(pinger Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pinger.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
