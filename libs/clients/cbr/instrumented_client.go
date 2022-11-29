package cbr

// Code generated by gowrap. DO NOT EDIT.
// template: ../../../.prom-gowrap.tmpl
// gowrap: http://github.com/hexdigest/gowrap

//go:generate gowrap gen -p github.com/brave-intl/bat-go/libs/clients/-i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go -l ""

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
		Name:       "cbr_client_duration_seconds",
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

// CreateIssuer implements Client
func (_d ClientWithPrometheus) CreateIssuer(ctx context.Context, issuer string, maxTokens int) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateIssuer", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateIssuer(ctx, issuer, maxTokens)
}

// CreateIssuerV3 implements Client
func (_d ClientWithPrometheus) CreateIssuerV3(ctx context.Context, createIssuerV3 IssuerRequest) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateIssuerV3", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateIssuerV3(ctx, createIssuerV3)
}

// GetIssuer implements Client
func (_d ClientWithPrometheus) GetIssuer(ctx context.Context, issuer string) (ip1 *IssuerResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuer", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuer(ctx, issuer)
}

// GetIssuerV2 implements Client
func (_d ClientWithPrometheus) GetIssuerV2(ctx context.Context, issuer string, cohort int16) (ip1 *IssuerResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuerV2", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuerV2(ctx, issuer, cohort)
}

// GetIssuerV3 implements Client
func (_d ClientWithPrometheus) GetIssuerV3(ctx context.Context, issuer string) (ip1 *IssuerResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuerV3", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuerV3(ctx, issuer)
}

// RedeemCredential implements Client
func (_d ClientWithPrometheus) RedeemCredential(ctx context.Context, issuer string, preimage string, signature string, payload string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "RedeemCredential", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RedeemCredential(ctx, issuer, preimage, signature, payload)
}

// RedeemCredentialV3 implements Client
func (_d ClientWithPrometheus) RedeemCredentialV3(ctx context.Context, issuer string, preimage string, signature string, payload string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "RedeemCredentialV3", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RedeemCredentialV3(ctx, issuer, preimage, signature, payload)
}

// RedeemCredentials implements Client
func (_d ClientWithPrometheus) RedeemCredentials(ctx context.Context, credentials []CredentialRedemption, payload string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "RedeemCredentials", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RedeemCredentials(ctx, credentials, payload)
}

// SignCredentials implements Client
func (_d ClientWithPrometheus) SignCredentials(ctx context.Context, issuer string, creds []string) (cp1 *CredentialsIssueResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "SignCredentials", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.SignCredentials(ctx, issuer, creds)
}
