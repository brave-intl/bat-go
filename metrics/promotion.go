package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// CountContributionsTotal counts the number of contributions made broken down by funding and type
	CountContributionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contributions_total",
			Help: "count of contributions made ( since last start ) broken down by funding and type",
		},
		[]string{"funding", "type"},
	)

	// CountContributionsBatTotal counts the total value of contributions in terms of bat ( since last start ) broken down by funding and type
	CountContributionsBatTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contributions_bat_total",
			Help: "total value of contributions in terms of bat ( since last start ) broken down by funding and type",
		},
		[]string{"funding", "type"},
	)

	// CountGrantsClaimedTotal counts the grants claimed, broken down by platform and type
	CountGrantsClaimedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grants_claimed_total",
			Help: "count of grants claimed ( since last start ) broken down by platform and type",
		},
		[]string{"platform", "type", "legacy"},
	)

	// CountGrantsClaimedBatTotal counts the total value of grants claimed in terms of bat ( since last start ) broken down by platform and type
	CountGrantsClaimedBatTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grants_claimed_bat_total",
			Help: "total value of grants claimed in terms of bat ( since last start ) broken down by platform and type",
		},
		[]string{"platform", "type", "legacy"},
	)

	// PromotionGetCount the number of promotions gotten
	PromotionGetCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "promotion_get_count",
			Help: "a count of the number of times the promotions were collected",
		},
		[]string{"filter", "migrate", "legacy"},
	)
	// PromotionExposureCount the count of promotion id
	PromotionExposureCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "promotion_exposure_count",
			Help: "a count of the number of times a single promotion was exposed to clients",
		},
		[]string{"id"},
	)
)
