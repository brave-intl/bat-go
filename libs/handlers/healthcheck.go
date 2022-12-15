package handlers

import (
	"net/http"
)

// HealthCheckResponseData - response structure for healthchecks
type HealthCheckResponseData struct {
	BuildTime string `json:"buildTime"`
	Commit    string `json:"commit"`
	Version   string `json:"version"`
	// service status is an accumulated map of service health structures mapped on service name
	ServiceStatus map[string]interface{} `json:"serviceStatus,omitempty"`
}

// HealthCheckResponse - response structure for healthchecks
type HealthCheckResponse struct {
	Data HealthCheckResponseData `json:"data"`
}

// HealthCheckHandler - function which generates a health check http.HandlerFunc
func HealthCheckHandler(version, buildTime, commit string, serviceStatus map[string]interface{}, check func() error) http.HandlerFunc {
	return AppHandler(
		func(w http.ResponseWriter, r *http.Request) *AppError {
			var ctx = r.Context()
			hcr := HealthCheckResponse{Data: HealthCheckResponseData{
				Commit:        commit,
				BuildTime:     buildTime,
				Version:       version,
				ServiceStatus: serviceStatus,
			}}

			if check != nil {
				if err := check(); err != nil {
					return &AppError{
						Message: "service is (partially) unavailable",
						Code:    500,
						Data:    hcr.Data,
					}
				}
			}

			return RenderContent(ctx, hcr, w, http.StatusOK)
		}).ServeHTTP
}
