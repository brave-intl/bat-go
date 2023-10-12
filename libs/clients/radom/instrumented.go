package radom

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type InstrumentedClient struct {
	name string
	cl   *Client
	vec  *prometheus.SummaryVec
}

// newInstrucmentedClient returns an instance of the Client decorated with prometheus summary metric.
func newInstrucmentedClient(name string, cl *Client) *InstrumentedClient {
	result := &InstrumentedClient{
		name: name,
		cl:   cl,
		vec: promauto.NewSummaryVec(prometheus.SummaryOpts{
			Name:       "client_duration_seconds",
			Help:       "client runtime duration and result",
			MaxAge:     time.Minute,
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
			[]string{"instance_name", "method", "result"},
		),
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
