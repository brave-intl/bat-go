package gemini

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../../../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/utils/clients/gemini -i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/cryptography"
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
		Name:       "gemini_client_duration_seconds",
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

// CheckTxStatus implements Client
func (_d ClientWithPrometheus) CheckTxStatus(ctx context.Context, APIKEY string, clientID string, txRef string) (pp1 *PayoutResult, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "CheckTxStatus", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CheckTxStatus(ctx, APIKEY, clientID, txRef)
}

// FetchAccountList implements Client
func (_d ClientWithPrometheus) FetchAccountList(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (aap1 *[]Account, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchAccountList", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchAccountList(ctx, APIKey, signer, payload)
}

// FetchBalances implements Client
func (_d ClientWithPrometheus) FetchBalances(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (bap1 *[]Balance, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchBalances", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchBalances(ctx, APIKey, signer, payload)
}

// UploadBulkPayout implements Client
func (_d ClientWithPrometheus) UploadBulkPayout(ctx context.Context, APIKey string, signer cryptography.HMACKey, payload string) (pap1 *[]PayoutResult, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "UploadBulkPayout", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UploadBulkPayout(ctx, APIKey, signer, payload)
}

// ValidateAccount implements Client
func (_d ClientWithPrometheus) ValidateAccount(ctx context.Context, verificationToken string, recipientID string) (s1 string, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "ValidateAccount", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ValidateAccount(ctx, verificationToken, recipientID)
}
