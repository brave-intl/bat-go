package service

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/wallet/service -i ReadOnlyDatastore -t ../../.prom-gowrap.tmpl -o instrumented_read_only_datastore.go

import (
	"time"

	"github.com/brave-intl/bat-go/wallet"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	uuid "github.com/satori/go.uuid"
)

// ReadOnlyDatastoreWithPrometheus implements ReadOnlyDatastore interface with all methods wrapped
// with Prometheus metrics
type ReadOnlyDatastoreWithPrometheus struct {
	base         ReadOnlyDatastore
	instanceName string
}

var readonlydatastoreDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "wallet_readonly_datastore_duration_seconds",
		Help:       "readonlydatastore runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"instance_name", "method", "result"})

// NewReadOnlyDatastoreWithPrometheus returns an instance of the ReadOnlyDatastore decorated with prometheus summary metric
func NewReadOnlyDatastoreWithPrometheus(base ReadOnlyDatastore, instanceName string) ReadOnlyDatastoreWithPrometheus {
	return ReadOnlyDatastoreWithPrometheus{
		base:         base,
		instanceName: instanceName,
	}
}

// GetWallet implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) GetWallet(id uuid.UUID) (ip1 *wallet.Info, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetWallet(id)
}

// RawDB implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) RawDB() (dp1 *sqlx.DB) {
	_since := time.Now()
	defer func() {
		result := "ok"
		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RawDB", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RawDB()
}
