package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// KafkaCertNotBefore checks when the kafka certificate becomes valid
	KafkaCertNotBefore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_before",
			Help: "Date when the kafka certificate becomes valid.",
		},
	)

	// KafkaCertNotAfter checks when the kafka certificate expires
	KafkaCertNotAfter = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_after",
			Help: "Date when the kafka certificate expires.",
		},
	)
)
