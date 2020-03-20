package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ClaimedGrantsCounter the number of grants that have been claimed
	ClaimedGrantsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "claimed_grants_total",
			Help: "Number of grants claimed since start.",
		},
		[]string{},
	)
	// RedeemedGrantsCounter the number of grants that were redeemed
	RedeemedGrantsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "redeemed_grants_total",
			Help: "Number of grants redeemed since start.",
		},
		[]string{"promotionId"},
	)
)
