package datastore

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/eyeshade/datastore -i Datastore -t ../../.prom-gowrap.tmpl -o instrumented_read_only_datastore.go

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/inputs"
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

// Commit implements Datastore
func (_d DatastoreWithPrometheus) Commit(ctx context.Context) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Commit", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.Commit(ctx)
}

// CountBallots implements Datastore
func (_d DatastoreWithPrometheus) CountBallots(ctx context.Context, p1 ...string) (bap1 *[]models.Ballot, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CountBallots", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CountBallots(ctx, p1...)
}

// FreezeSurveyors implements Datastore
func (_d DatastoreWithPrometheus) FreezeSurveyors(ctx context.Context, p1 ...string) (sap1 *[]models.Surveyor, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "FreezeSurveyors", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.FreezeSurveyors(ctx, p1...)
}

// GetAccountEarnings implements Datastore
func (_d DatastoreWithPrometheus) GetAccountEarnings(ctx context.Context, options models.AccountEarningsOptions) (aap1 *[]models.AccountEarnings, err error) {
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
func (_d DatastoreWithPrometheus) GetAccountSettlementEarnings(ctx context.Context, options models.AccountSettlementEarningsOptions) (aap1 *[]models.AccountSettlementEarnings, err error) {
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

// GetActiveCountryGroups implements Datastore
func (_d DatastoreWithPrometheus) GetActiveCountryGroups(ctx context.Context) (rap1 *[]models.ReferralGroup, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetActiveCountryGroups", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetActiveCountryGroups(ctx)
}

// GetBalances implements Datastore
func (_d DatastoreWithPrometheus) GetBalances(ctx context.Context, accountIDs []string) (bap1 *[]models.Balance, err error) {
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

// GetBallotsByID implements Datastore
func (_d DatastoreWithPrometheus) GetBallotsByID(ctx context.Context, p1 ...string) (bap1 *[]models.Ballot, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetBallotsByID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetBallotsByID(ctx, p1...)
}

// GetFreezableSurveyors implements Datastore
func (_d DatastoreWithPrometheus) GetFreezableSurveyors(ctx context.Context, i1 int) (sap1 *[]models.Surveyor, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetFreezableSurveyors", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetFreezableSurveyors(ctx, i1)
}

// GetGrantStats implements Datastore
func (_d DatastoreWithPrometheus) GetGrantStats(ctx context.Context, options models.GrantStatOptions) (gp1 *models.GrantStat, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetGrantStats", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetGrantStats(ctx, options)
}

// GetPending implements Datastore
func (_d DatastoreWithPrometheus) GetPending(ctx context.Context, accountIDs []string) (pap1 *[]models.PendingTransaction, err error) {
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

// GetReferralGroups implements Datastore
func (_d DatastoreWithPrometheus) GetReferralGroups(ctx context.Context, activeAt inputs.Time) (rap1 *[]models.ReferralGroup, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetReferralGroups", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetReferralGroups(ctx, activeAt)
}

// GetSettlementStats implements Datastore
func (_d DatastoreWithPrometheus) GetSettlementStats(ctx context.Context, options models.SettlementStatOptions) (sp1 *models.SettlementStat, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetSettlementStats", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetSettlementStats(ctx, options)
}

// GetTransactions implements Datastore
func (_d DatastoreWithPrometheus) GetTransactions(ctx context.Context, constraints ...map[string]string) (tap1 *[]models.Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetTransactions(ctx, constraints...)
}

// GetTransactionsByAccount implements Datastore
func (_d DatastoreWithPrometheus) GetTransactionsByAccount(ctx context.Context, accountID string, txTypes []string) (tap1 *[]models.Transaction, err error) {
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

// GetTransactionsByID implements Datastore
func (_d DatastoreWithPrometheus) GetTransactionsByID(ctx context.Context, p1 ...string) (tap1 *[]models.Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetTransactionsByID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetTransactionsByID(ctx, p1...)
}

// InsertBallots implements Datastore
func (_d DatastoreWithPrometheus) InsertBallots(ctx context.Context, ballots *[]models.Ballot) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertBallots", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertBallots(ctx, ballots)
}

// InsertConvertableTransactions implements Datastore
func (_d DatastoreWithPrometheus) InsertConvertableTransactions(ctx context.Context, txs []models.ConvertableTransaction) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertConvertableTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertConvertableTransactions(ctx, txs)
}

// InsertSurveyors implements Datastore
func (_d DatastoreWithPrometheus) InsertSurveyors(ctx context.Context, surveyors *[]models.Surveyor) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertSurveyors", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertSurveyors(ctx, surveyors)
}

// InsertTransactions implements Datastore
func (_d DatastoreWithPrometheus) InsertTransactions(ctx context.Context, txs *[]models.Transaction) (err error) {
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

// InsertVotes implements Datastore
func (_d DatastoreWithPrometheus) InsertVotes(ctx context.Context, votes []models.Vote) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertVotes", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertVotes(ctx, votes)
}

// Migrate implements Datastore
func (_d DatastoreWithPrometheus) Migrate(p1 ...uint) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Migrate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.Migrate(p1...)
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

// ResolveConnection implements Datastore
func (_d DatastoreWithPrometheus) ResolveConnection(ctx context.Context) (c2 context.Context, tp1 *sqlx.Tx, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "ResolveConnection", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ResolveConnection(ctx)
}

// Rollback implements Datastore
func (_d DatastoreWithPrometheus) Rollback(ctx context.Context) {
	_since := time.Now()
	defer func() {
		result := "ok"
		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "Rollback", result).Observe(time.Since(_since).Seconds())
	}()
	_d.base.Rollback(ctx)
	return
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

// SeedDB implements Datastore
func (_d DatastoreWithPrometheus) SeedDB(ctx context.Context) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "SeedDB", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.SeedDB(ctx)
}

// SetVoteFees implements Datastore
func (_d DatastoreWithPrometheus) SetVoteFees(ctx context.Context, p1 ...string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "SetVoteFees", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.SetVoteFees(ctx, p1...)
}

// WithTx implements Datastore
func (_d DatastoreWithPrometheus) WithTx(ctx context.Context) (c2 context.Context, tp1 *sqlx.Tx, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "WithTx", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.WithTx(ctx)
}
