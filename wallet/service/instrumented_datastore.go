package service

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/wallet/service -i Datastore -t https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus -o instrumeted_datastore.go

import (
	"time"

	"github.com/brave-intl/bat-go/wallet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	uuid "github.com/satori/go.uuid"
)

// DatastoreWithPrometheus implements Datastore interface with all methods wrapped
// with Prometheus metrics
type DatastoreWithPrometheus struct {
	base         Datastore
	instanceName string
}

var datastoreDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "datastore_duration_seconds",
		Help:       "datastore runtime duration and result",
		MaxAge:     time.Minute,
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
	[]string{"instance_name", "method", "result"})

// NewDatastoreWithPrometheus returns an instance of the Datastore decorated with prometheus summary metric
func NewDatastoreWithPrometheus(base Datastore, instanceName string) DatastoreWithPrometheus {
	return DatastoreWithPrometheus{
		base:         base,
		instanceName: instanceName,
	}
}

// GetWallet implements Datastore
func (_d DatastoreWithPrometheus) GetWallet(id uuid.UUID) (ip1 *wallet.Info, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetWallet(id)
}

// UpsertWallet implements Datastore
func (_d DatastoreWithPrometheus) UpsertWallet(wallet *wallet.Info) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "UpsertWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UpsertWallet(wallet)
}
