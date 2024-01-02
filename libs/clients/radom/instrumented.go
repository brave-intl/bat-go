package radom

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type InstrumentedClient struct {
	name string
	cl   *Client
	vec  *prometheus.SummaryVec
}

// newInstrumentedClient returns an instance of the Client decorated with prometheus summary metric.
// This function panics if it cannot register the metric.
func newInstrumentedClient(name string, cl *Client) *InstrumentedClient {
	v := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       "radom_client_duration_seconds",
		Help:       "client runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
		[]string{"instance_name", "method", "result"},
	)
	prometheus.MustRegister(v)

	result := &InstrumentedClient{
		name: name,
		cl:   cl,
		vec:  v,
	}

	return result
}

func (_d *InstrumentedClient) CreateCheckoutSession(ctx context.Context, cp1 *CheckoutSessionRequest) (cp2 *CheckoutSessionResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		_d.vec.WithLabelValues(_d.name, "CreateCheckoutSession", result).Observe(time.Since(_since).Seconds())
	}()

	return _d.cl.CreateCheckoutSession(ctx, cp1)
}
