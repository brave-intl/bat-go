package grant

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/grant -i ReadOnlyDatastore -t ../.prom-gowrap.tmpl -o instrumented_read_only_datastore.go

import (
	"time"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/wallet"
	migrate "github.com/golang-migrate/migrate/v4"
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
		Name:       "grant_readonly_datastore_duration_seconds",
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

// GetGrantsOrderedByExpiry implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) (ga1 []Grant, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetGrantsOrderedByExpiry", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetGrantsOrderedByExpiry(wallet, promotionType)
}

// GetPromotion implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) GetPromotion(promotionID uuid.UUID) (pp1 *promotion.Promotion, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetPromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetPromotion(promotionID)
}

// Migrate implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) Migrate() (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Migrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.Migrate()
}

// NewMigrate implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) NewMigrate() (mp1 *migrate.Migrate, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "NewMigrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.NewMigrate()
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

// RollbackTx implements ReadOnlyDatastore
func (_d ReadOnlyDatastoreWithPrometheus) RollbackTx(tx *sqlx.Tx) {
	_since := time.Now()
	defer func() {
		result := "ok"
		readonlydatastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RollbackTx", result).Observe(time.Since(_since).Seconds())
	}()
	_d.base.RollbackTx(tx)
	return
}
