package metric

import (
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	status      = "status"
	countryCode = "country_code"
	success     = "success"
	failure     = "failure"
)

type Metric struct {
	cntLinkZP                *prometheus.CounterVec
	cntAccValidateGemini     *prometheus.CounterVec
	cntDocTypeByIssuingCntry *prometheus.CounterVec
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

	accValidate := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "count_gemini_wallet_account_validation",
			Help: "Counts the number of gemini wallets requesting account validation partitioned by country code",
		},
		[]string{countryCode, status},
	)
	prometheus.MustRegister(accValidate)

	cntDocTypeByIssuingCntry := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "count_gemini_document_type_by_issuing_country",
			Help: "Counts the number document types being used for KYC broken down by country",
		},
		[]string{"document_type", "issuing_country"},
	)
	prometheus.MustRegister(cntDocTypeByIssuingCntry)

	return &Metric{
		cntLinkZP:                clzp,
		cntAccValidateGemini:     accValidate,
		cntDocTypeByIssuingCntry: cntDocTypeByIssuingCntry,
	}
}

func (m *Metric) LinkSuccessZP(cc string) {
	m.cntLinkZP.With(prometheus.Labels{
		countryCode: cc,
		status:      success,
	}).Inc()
}

func (m *Metric) LinkFailureZP(cc string) {
	m.cntLinkZP.With(prometheus.Labels{
		countryCode: cc,
		status:      failure,
	}).Inc()
}

func (m *Metric) LinkFailureGemini(cc string) {
	m.cntAccValidateGemini.With(prometheus.Labels{
		countryCode: cc,
		status:      failure,
	}).Inc()
}

func (m *Metric) LinkSuccessGemini(cc string) {
	m.cntAccValidateGemini.With(prometheus.Labels{
		countryCode: cc,
		status:      success,
	}).Inc()
}

func (m *Metric) CountDocTypeByIssuingCntry(validDocs []gemini.ValidDocument) {
	for i := range validDocs {
		m.cntDocTypeByIssuingCntry.With(prometheus.Labels{
			"document_type":   validDocs[i].Type,
			"issuing_country": validDocs[i].IssuingCountry,
		}).Inc()
	}
}
