package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	prometheus.MustRegister(
		// old
		RedeemedGrantsCounter,
		ClaimedGrantsCounter,
		// kafka
		KafkaCertNotBefore,
		KafkaCertNotAfter,
		// promotion
		CountContributionsTotal,
		CountContributionsBatTotal,
		CountGrantsClaimedTotal,
		CountGrantsClaimedBatTotal,
		PromotionGetCount,
		PromotionExposureCount,
	)
}

// InitStatsCollector creates a stats collector
func InitStatsCollector(prefix string, db StatsGetter) {
	// setup instrumentation using sqlstats
	// Create a new collector, the name will be used as a label on the metrics
	collector := NewStatsCollector(prefix, db)
	// Register it with Prometheus
	err := prometheus.Register(collector)

	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		// take old collector, and add the new db
		if sc, ok := ae.ExistingCollector.(*StatsCollector); ok {
			sc.AddStatsGetter(prefix, db)
		}
	}
}
