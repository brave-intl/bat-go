package kafka

import (
	"context"
	"crypto/x509"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// kafkaCertNotAfter checks when the kafka certificate becomes valid
	kafkaCertNotBefore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_before",
			Help: "Date when the kafka certificate becomes valid.",
		},
	)

	// kafkaCertNotAfter checks when the kafka certificate expires
	kafkaCertNotAfter = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_after",
			Help: "Date when the kafka certificate expires.",
		},
	)
)

// InstrumentKafka - setup instrumentation and metrics around our kafka connection
func InstrumentKafka(ctx context.Context) {
	logger := logging.Logger(ctx, "kafka.InstrumentKafka")

	cert, ok := ctx.Value(appctx.Kafka509CertCTXKey).(*x509.Certificate)
	if !ok {
		// no cert on context
		logger.Info().Msg("no kafka cert on context, not initializing kafka instrumentation")
		return
	}

	err := prometheus.Register(kafkaCertNotAfter)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		kafkaCertNotAfter = ae.ExistingCollector.(prometheus.Gauge)
	}

	err = prometheus.Register(kafkaCertNotBefore)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		kafkaCertNotBefore = ae.ExistingCollector.(prometheus.Gauge)
	}

	logger.Info().Msg("registered kafka cert not before and not after prom metrics")

	kafkaCertNotBefore.Set(float64(cert.NotBefore.Unix()))
	kafkaCertNotAfter.Set(float64(cert.NotAfter.Unix()))

	logger.Info().Msg("set values for kafka cert not before and not after prom metrics")
}
