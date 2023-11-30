package metric

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	status      = "status"
	countryCode = "country_code"
)

type Metric struct {
	cntLinkZP *prometheus.CounterVec
}

// New returns a new metric.Metric.
// New panics if it cannot register any of the metrics.
func New() *Metric {
	clzp := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "count_link_zebpay",
		Help: "Counts the number of successful and failed ZebPay linkings partitioned by country code",
	},
		[]string{status, countryCode},
	)
	prometheus.MustRegister(clzp)

	return &Metric{cntLinkZP: clzp}
}

func (m *Metric) LinkSuccessZP(cc string) {
	const success = "success"
	m.cntLinkZP.With(prometheus.Labels{
		status:      success,
		countryCode: cc,
	}).Inc()
}

func (m *Metric) LinkFailureZP(cc string) {
	const failure = "failure"
	m.cntLinkZP.With(prometheus.Labels{
		status:      failure,
		countryCode: cc,
	}).Inc()
}
