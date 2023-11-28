package metric

import "github.com/prometheus/client_golang/prometheus"

const (
	cntLinkZP     = "count_link_zebpay"
	cntLinkHelpZP = "Counts the number of successful and failed ZebPay linkings partitioned by country code"
	status        = "status"
	countryCode   = "country_code"
)

type Metric struct {
	cntLinkZP *prometheus.CounterVec
}

func New() *Metric {
	clzp := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: cntLinkZP,
		Help: cntLinkHelpZP,
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
	})
}

func (m *Metric) LinkFailureZP(cc string) {
	const failure = "failure"

	m.cntLinkZP.With(prometheus.Labels{
		status:      failure,
		countryCode: cc,
	})
}
