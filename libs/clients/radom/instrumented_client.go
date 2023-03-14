// Code generated by gowrap. DO NOT EDIT.
// template: ../../../.prom-gowrap.tmpl
// gowrap: http://github.com/hexdigest/gowrap

package radom

//go:generate gowrap gen -p github.com/brave-intl/bat-go/libs/clients/-i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go -l ""

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClientWithPrometheus implements Client interface with all methods wrapped
// with Prometheus metrics
type ClientWithPrometheus struct {
	base         Client
	instanceName string
}

var clientDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "client_duration_seconds",
		Help:       "client runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"instance_name", "method", "result"})

// NewClientWithPrometheus returns an instance of the Client decorated with prometheus summary metric
func NewClientWithPrometheus(base Client, instanceName string) ClientWithPrometheus {
	return ClientWithPrometheus{
		base:         base,
		instanceName: instanceName,
	}
}

// CreateCheckoutSession implements Client
func (_d ClientWithPrometheus) CreateCheckoutSession(cp1 *CheckoutSessionRequest) (cp2 *CheckoutSessionResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateCheckoutSession", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateCheckoutSession(cp1)
}
