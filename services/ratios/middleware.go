package ratios

import (
	"net/http"
	"os"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	xBraveKeyHeaderPresentCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "x_brave_key_header_count",
			Help: "A count of the requests by whether the x-brave-key header is present",
		},
		[]string{"handler", "present"},
	)
)

func init() {
	if err := prometheus.Register(xBraveKeyHeaderPresentCounter); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			xBraveKeyHeaderPresentCounter = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}
}

// XBraveHeaderInstrumentHandler instruments an http.Handler to capture
// data relevant to the ratios service
func XBraveHeaderInstrumentHandler(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-brave-key")
		expectedKey := os.Getenv("X_BRAVE_KEY")

		var present bool
		if expectedKey == "" {
			present = key != ""
		} else {
			present = key == expectedKey
		}

		xBraveKeyHeaderPresentCounter.With(prometheus.Labels{
			"present": strconv.FormatBool(present),
			"handler": name,
		}).Inc()

		next.ServeHTTP(w, r)
	})
}
