package bitflyer

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../../../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/utils/clients/bitflyer -i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClientWithPrometheus implements Client interface with all methods wrapped
// with Prometheus metrics
type ClientWithPrometheus struct {
	base         Client
	instanceName string
}

var clientDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "bitflyer_client_duration_seconds",
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

// CheckPayoutStatus implements Client
func (_d ClientWithPrometheus) CheckPayoutStatus(ctx context.Context, payload WithdrawToDepositIDBulkPayload) (wp1 *WithdrawToDepositIDBulkResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "CheckPayoutStatus", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CheckPayoutStatus(ctx, payload)
}

// FetchQuote implements Client
func (_d ClientWithPrometheus) FetchQuote(ctx context.Context, productCode string) (qp1 *Quote, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchQuote", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchQuote(ctx, productCode)
}

// RefreshToken implements Client
func (_d ClientWithPrometheus) RefreshToken(ctx context.Context, payload TokenPayload) (tp1 *TokenResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "RefreshToken", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RefreshToken(ctx, payload)
}

// SetAuthToken implements Client
func (_d ClientWithPrometheus) SetAuthToken(authToken string) {
	_since := time.Now()
	defer func() {
		result := "ok"
		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "SetAuthToken", result).Observe(time.Since(_since).Seconds())
	}()
	_d.base.SetAuthToken(authToken)
	return
}

// UploadBulkPayout implements Client
func (_d ClientWithPrometheus) UploadBulkPayout(ctx context.Context, payload WithdrawToDepositIDBulkPayload) (wp1 *WithdrawToDepositIDBulkResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "UploadBulkPayout", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UploadBulkPayout(ctx, payload)
}
