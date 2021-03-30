package eyeshade

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/eyeshade -i Datastore -t ../.prom-gowrap.tmpl -o instrumented_read_only_datastore.go

import (
	"context"
	"time"

	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

// GetAccountEarnings implements Datastore
func (_d DatastoreWithPrometheus) GetAccountEarnings(ctx context.Context, options AccountEarningsOptions) (aap1 *[]AccountEarnings, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAccountEarnings", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAccountEarnings(ctx, options)
}

// Migrate implements Datastore
func (_d DatastoreWithPrometheus) Migrate(currentMigrationVersion uint) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Migrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.Migrate(currentMigrationVersion)
}

// NewMigrate implements Datastore
func (_d DatastoreWithPrometheus) NewMigrate() (mp1 *migrate.Migrate, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "NewMigrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.NewMigrate()
}

// RawDB implements Datastore
func (_d DatastoreWithPrometheus) RawDB() (dp1 *sqlx.DB) {
	_since := time.Now()
	defer func() {
		result := "ok"
		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RawDB", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RawDB()
}

// RollbackTx implements Datastore
func (_d DatastoreWithPrometheus) RollbackTx(tx *sqlx.Tx) {
	_since := time.Now()
	defer func() {
		result := "ok"
		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RollbackTx", result).Observe(time.Since(_since).Seconds())
	}()
	_d.base.RollbackTx(tx)
	return
}

// RollbackTxAndHandle implements Datastore
func (_d DatastoreWithPrometheus) RollbackTxAndHandle(tx *sqlx.Tx) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RollbackTxAndHandle", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RollbackTxAndHandle(tx)
}
