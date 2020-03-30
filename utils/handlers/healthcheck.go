package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// HealthCheckResponse - response structure for healthchecks
type HealthCheckResponse struct {
	BuildTime string `json:"build_time"`
	Commit    string `json:"commit"`
	Version   string `json:"version"`
}

// RenderJSON - helper to render a HealthCheckResponse as Json to an http.ResponseWriter
func (hcr HealthCheckResponse) RenderJSON(w http.ResponseWriter) error {
	body, err := json.Marshal(hcr)
	if err != nil {
		return fmt.Errorf("failed to marshal response in render json: %w", err)
	}
	w.WriteHeader(200)
	if _, err := w.Write(body); err != nil {
		log.Printf("failed to write response to writer: %s", err)
	}
	return nil
}

// HealthCheckHandler - function which generates a health check http.HandlerFunc
func HealthCheckHandler(version, buildTime, commit string) http.HandlerFunc {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			hcr := HealthCheckResponse{
				Commit:    commit,
				BuildTime: buildTime,
				Version:   version,
			}
			if err := hcr.RenderJSON(w); err != nil {
				w.WriteHeader(500)
				if _, err := w.Write([]byte("unhealthy")); err != nil {
					log.Printf("failed to write response to writer: %s", err)
				}
			}
		})
}

// PingHandler - used with pingdom check
func PingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(".")); err != nil {
		log.Printf("failed to write response to writer: %s", err)
	}
}
