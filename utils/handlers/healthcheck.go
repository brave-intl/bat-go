package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/utils/logging"
)

// HealthCheckResponse - response structure for healthchecks
type HealthCheckResponse struct {
	BuildTime string `json:"build_time"`
	Commit    string `json:"commit"`
	Version   string `json:"version"`
}

// RenderJSON - helper to render a HealthCheckResponse as Json to an http.ResponseWriter
func (hcr HealthCheckResponse) RenderJSON(ctx context.Context, w http.ResponseWriter) error {
	logger := logging.Logger(ctx, "handlers.HealthCheckResponse.RenderJSON")
	body, err := json.Marshal(hcr)
	if err != nil {
		return fmt.Errorf("failed to marshal response in render json: %w", err)
	}
	w.WriteHeader(200)
	if _, err := w.Write(body); err != nil {
		logger.Error().Err(err).Msg("failed to write response to writer")
	}
	return nil
}

// HealthCheckHandler - function which generates a health check http.HandlerFunc
func HealthCheckHandler(version, buildTime, commit string) http.HandlerFunc {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var ctx = r.Context()
			logger := logging.Logger(ctx, "handlers.HealthCheckHandler")

			hcr := HealthCheckResponse{
				Commit:    commit,
				BuildTime: buildTime,
				Version:   version,
			}
			if err := hcr.RenderJSON(ctx, w); err != nil {
				w.WriteHeader(500)
				if _, err := w.Write([]byte("unhealthy")); err != nil {
					logger.Error().Err(err).Msg("failed to write response to writer")
				}
			}
		})
}
