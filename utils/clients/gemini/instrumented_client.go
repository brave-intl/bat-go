package gemini

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../../../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/utils/clients/gemini -i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go

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
		Name:       "client_duration_seconds",
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

// FetchAccountList implements Client
func (_d ClientWithPrometheus) FetchAccountList(ctx context.Context, request PrivateRequest) (aap1 *[]Account, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchAccountList", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchAccountList(ctx, request)
}

// FetchBalances implements Client
func (_d ClientWithPrometheus) FetchBalances(ctx context.Context, request PrivateRequest) (bap1 *[]Balance, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchBalances", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchBalances(ctx, request)
}

// UploadBulkPayout implements Client
func (_d ClientWithPrometheus) UploadBulkPayout(ctx context.Context, request PrivateRequest) (pap1 *[]PayoutResult, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "UploadBulkPayout", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UploadBulkPayout(ctx, request)
}
