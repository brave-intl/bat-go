package reputation

// Code generated by gowrap. DO NOT EDIT.
// template: ../../../.prom-gowrap.tmpl
// gowrap: http://github.com/hexdigest/gowrap

//go:generate gowrap gen -p github.com/brave-intl/bat-go/utils/clients/reputation -i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go -l ""

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// ClientWithPrometheus implements Client interface with all methods wrapped
// with Prometheus metrics
type ClientWithPrometheus struct {
	base         Client
	instanceName string
}

var clientDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "reputation_client_duration_seconds",
		Help:       "client runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"instance_name", "method", "result"})

// NewClientWithPrometheus returns an instance of the Client decorated with prometheus summary metric
func NewClientWithPrometheus(base Client, instanceName string) ClientWithPrometheus {
	return ClientWithPrometheus{
		base:         base,
		instanceName: instanceName,
	}
}

// IsDrainReputable implements Client
func (_d ClientWithPrometheus) IsDrainReputable(ctx context.Context, id uuid.UUID, promotionID uuid.UUID, withdrawAmount decimal.Decimal) (b1 bool, ia1 []int, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "IsDrainReputable", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.IsDrainReputable(ctx, id, promotionID, withdrawAmount)
}

// IsWalletAdsReputable implements Client
func (_d ClientWithPrometheus) IsWalletAdsReputable(ctx context.Context, id uuid.UUID, platform string) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "IsWalletAdsReputable", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.IsWalletAdsReputable(ctx, id, platform)
}

// IsWalletOnPlatform implements Client
func (_d ClientWithPrometheus) IsWalletOnPlatform(ctx context.Context, id uuid.UUID, platform string) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "IsWalletOnPlatform", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.IsWalletOnPlatform(ctx, id, platform)
}

// IsWalletReputable implements Client
func (_d ClientWithPrometheus) IsWalletReputable(ctx context.Context, id uuid.UUID, platform string) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "IsWalletReputable", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.IsWalletReputable(ctx, id, platform)
}
