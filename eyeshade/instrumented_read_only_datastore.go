package eyeshade

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/eyeshade -i Datastore -t ../.prom-gowrap.tmpl -o instrumented_read_only_datastore.go

import (
	"context"
	"database/sql"
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

// GetAccountSettlementEarnings implements Datastore
func (_d DatastoreWithPrometheus) GetAccountSettlementEarnings(ctx context.Context, options AccountSettlementEarningsOptions) (aap1 *[]AccountSettlementEarnings, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAccountSettlementEarnings", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAccountSettlementEarnings(ctx, options)
}

// GetBalances implements Datastore
func (_d DatastoreWithPrometheus) GetBalances(ctx context.Context, accountIDs []string) (bap1 *[]Balance, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetBalances", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetBalances(ctx, accountIDs)
}

// GetPending implements Datastore
func (_d DatastoreWithPrometheus) GetPending(ctx context.Context, accountIDs []string) (pap1 *[]PendingTransaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetPending", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetPending(ctx, accountIDs)
}

// GetTransactionsByAccount implements Datastore
func (_d DatastoreWithPrometheus) GetTransactionsByAccount(ctx context.Context, accountID string, txTypes []string) (tap1 *[]Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetTransactionsByAccount", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetTransactionsByAccount(ctx, accountID, txTypes)
}

// InsertFromReferrals implements Datastore
func (_d DatastoreWithPrometheus) InsertFromReferrals(ctx context.Context, txs []Referral) (r1 sql.Result, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertFromReferrals", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertFromReferrals(ctx, txs)
}

// InsertFromSettlements implements Datastore
func (_d DatastoreWithPrometheus) InsertFromSettlements(ctx context.Context, txs []Settlement) (r1 sql.Result, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertFromSettlements", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertFromSettlements(ctx, txs)
}

// InsertFromVoting implements Datastore
func (_d DatastoreWithPrometheus) InsertFromVoting(ctx context.Context, txs []Votes) (r1 sql.Result, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertFromVoting", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertFromVoting(ctx, txs)
}

// InsertTransactions implements Datastore
func (_d DatastoreWithPrometheus) InsertTransactions(ctx context.Context, txs *[]Transaction) (r1 sql.Result, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertTransactions(ctx, txs)
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
