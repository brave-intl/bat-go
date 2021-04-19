package wallet

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/wallet -i Datastore -t ../.prom-gowrap.tmpl -o instrumented_datastore.go

import (
	"context"
	"time"

	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	migrate "github.com/golang-migrate/migrate/v4"
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
		Name:       "wallet_datastore_duration_seconds",
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

// GetByProviderLinkingID implements Datastore
func (_d DatastoreWithPrometheus) GetByProviderLinkingID(ctx context.Context, providerLinkingID uuid.UUID) (iap1 *[]walletutils.Info, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetByProviderLinkingID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetByProviderLinkingID(ctx, providerLinkingID)
}

// GetLinkingLimitInfo implements Datastore
func (_d DatastoreWithPrometheus) GetLinkingLimitInfo(ctx context.Context, providerLinkingID string) (l1 LinkingInfo, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetLinkingLimitInfo", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetLinkingLimitInfo(ctx, providerLinkingID)
}

// GetWallet implements Datastore
func (_d DatastoreWithPrometheus) GetWallet(ctx context.Context, ID uuid.UUID) (ip1 *walletutils.Info, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetWallet(ctx, ID)
}

// GetWalletByPublicKey implements Datastore
func (_d DatastoreWithPrometheus) GetWalletByPublicKey(ctx context.Context, s1 string) (ip1 *walletutils.Info, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetWalletByPublicKey", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetWalletByPublicKey(ctx, s1)
}

// IncreaseLinkingLimit implements Datastore
func (_d DatastoreWithPrometheus) IncreaseLinkingLimit(ctx context.Context, providerLinkingID uuid.UUID) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "IncreaseLinkingLimit", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.IncreaseLinkingLimit(ctx, providerLinkingID)
}

// InsertBitFlyerRequestID implements Datastore
func (_d DatastoreWithPrometheus) InsertBitFlyerRequestID(ctx context.Context, requestID string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertBitFlyerRequestID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertBitFlyerRequestID(ctx, requestID)
}

// InsertWallet implements Datastore
func (_d DatastoreWithPrometheus) InsertWallet(ctx context.Context, wallet *walletutils.Info) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertWallet(ctx, wallet)
}

// LinkWallet implements Datastore
func (_d DatastoreWithPrometheus) LinkWallet(ctx context.Context, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, depositProvider string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "LinkWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.LinkWallet(ctx, ID, providerID, providerLinkingID, anonymousAddress, depositProvider)
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

// TxLinkWalletInfo implements Datastore
func (_d DatastoreWithPrometheus) TxLinkWalletInfo(ctx context.Context, tx *sqlx.Tx, ID string, providerID string, providerLinkingID uuid.UUID, anonymousAddress *uuid.UUID, pda string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "TxLinkWalletInfo", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.TxLinkWalletInfo(ctx, tx, ID, providerID, providerLinkingID, anonymousAddress, pda)
}

// UpsertWallet implements Datastore
func (_d DatastoreWithPrometheus) UpsertWallet(ctx context.Context, wallet *walletutils.Info) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "UpsertWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UpsertWallet(ctx, wallet)
}
