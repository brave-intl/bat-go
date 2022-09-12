package ratios

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"os"
	"strconv"
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
	prometheus.Register(xBraveKeyHeaderPresentCounter)
}

// RatiosXBraveHeaderInstrumentHandler instruments an http.Handler to capture
// data relevant to the ratios service
func RatiosXBraveHeaderInstrumentHandler(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("x-brave-key")
		expectedKey := os.Getenv("X_BRAVE_KEY")
		matches := key == expectedKey
		xBraveKeyHeaderPresentCounter.With(prometheus.Labels{
			"present": strconv.FormatBool(matches),
			"handler": name,
		}).Inc()
	})
}
