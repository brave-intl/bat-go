package grant

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/grant -i Datastore -t https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus -o instrumented_datastore.go

import (
	"time"

	promotion "github.com/brave-intl/bat-go/promotion"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
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
		Name:       "grant_datastore_duration_seconds",
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

// ClaimPromotionForWallet implements Datastore
func (_d DatastoreWithPrometheus) ClaimPromotionForWallet(promo *promotion.Promotion, wallet *wallet.Info) (cp1 *promotion.Claim, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "ClaimPromotionForWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ClaimPromotionForWallet(promo, wallet)
}

// GetGrantsOrderedByExpiry implements Datastore
func (_d DatastoreWithPrometheus) GetGrantsOrderedByExpiry(wallet wallet.Info, promotionType string) (ga1 []Grant, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetGrantsOrderedByExpiry", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetGrantsOrderedByExpiry(wallet, promotionType)
}

// GetPromotion implements Datastore
func (_d DatastoreWithPrometheus) GetPromotion(promotionID uuid.UUID) (pp1 *promotion.Promotion, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetPromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetPromotion(promotionID)
}

// Migrate implements Datastore
func (_d DatastoreWithPrometheus) Migrate() (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Migrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.Migrate()
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

// RedeemGrantForWallet implements Datastore
func (_d DatastoreWithPrometheus) RedeemGrantForWallet(grant Grant, wallet wallet.Info) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RedeemGrantForWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RedeemGrantForWallet(grant, wallet)
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
