package coingecko

// Code generated by gowrap. DO NOT EDIT.
// template: ../../../.prom-gowrap.tmpl
// gowrap: http://github.com/hexdigest/gowrap

//go:generate gowrap gen -p github.com/brave-intl/bat-go/utils/clients/coingecko -i Client -t ../../../.prom-gowrap.tmpl -o instrumented_client.go -l ""

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
		Name:       "coingecko_client_duration_seconds",
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

// FetchCoinList implements Client
func (_d ClientWithPrometheus) FetchCoinList(ctx context.Context, includePlatform bool) (cp1 *CoinListResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchCoinList", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchCoinList(ctx, includePlatform)
}

// FetchMarketChart implements Client
func (_d ClientWithPrometheus) FetchMarketChart(ctx context.Context, id string, vsCurrency string, days float32) (mp1 *MarketChartResponse, t1 time.Time, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchMarketChart", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchMarketChart(ctx, id, vsCurrency, days)
}

// FetchSimplePrice implements Client
func (_d ClientWithPrometheus) FetchSimplePrice(ctx context.Context, ids string, vsCurrencies string, include24hrChange bool) (sp1 *SimplePriceResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchSimplePrice", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchSimplePrice(ctx, ids, vsCurrencies, include24hrChange)
}

// FetchSupportedVsCurrencies implements Client
func (_d ClientWithPrometheus) FetchSupportedVsCurrencies(ctx context.Context) (sp1 *SupportedVsCurrenciesResponse, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		clientDurationSummaryVec.WithLabelValues(_d.instanceName, "FetchSupportedVsCurrencies", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FetchSupportedVsCurrencies(ctx)
}
