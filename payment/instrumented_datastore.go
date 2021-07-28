package payment

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using ../.prom-gowrap.tmpl template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/payment -i Datastore -t ../.prom-gowrap.tmpl -o instrumented_datastore.go

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/inputs"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// DatastoreWithPrometheus implements Datastore interface with all methods wrapped
// with Prometheus metrics
type DatastoreWithPrometheus struct {
	base         Datastore
	instanceName string
}

var datastoreDurationSummaryVec = promauto.NewSummaryVec(
	prometheus.SummaryOpts{
		Name:       "payment_datastore_duration_seconds",
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

// CheckExpiredCheckoutSession implements Datastore
func (_d DatastoreWithPrometheus) CheckExpiredCheckoutSession(u1 uuid.UUID) (b1 bool, s1 string, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CheckExpiredCheckoutSession", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CheckExpiredCheckoutSession(u1)
}

// CommitVote implements Datastore
func (_d DatastoreWithPrometheus) CommitVote(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CommitVote", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CommitVote(ctx, vr, tx)
}

// CreateKey implements Datastore
func (_d DatastoreWithPrometheus) CreateKey(merchant string, name string, encryptedSecretKey string, nonce string) (kp1 *Key, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateKey", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateKey(merchant, name, encryptedSecretKey, nonce)
}

// CreateOrder implements Datastore
func (_d DatastoreWithPrometheus) CreateOrder(totalPrice decimal.Decimal, merchantID string, status string, currency string, location string, orderItems []OrderItem, allowedPaymentMethods *Methods) (op1 *Order, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateOrder", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateOrder(totalPrice, merchantID, status, currency, location, orderItems, allowedPaymentMethods)
}

// CreateTransaction implements Datastore
func (_d DatastoreWithPrometheus) CreateTransaction(orderID uuid.UUID, externalTransactionID string, status string, currency string, kind string, amount decimal.Decimal) (tp1 *Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateTransaction", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateTransaction(orderID, externalTransactionID, status, currency, kind, amount)
}

// DeleteKey implements Datastore
func (_d DatastoreWithPrometheus) DeleteKey(id uuid.UUID, delaySeconds int) (kp1 *Key, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "DeleteKey", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.DeleteKey(id, delaySeconds)
}

// DeleteOrderCreds implements Datastore
func (_d DatastoreWithPrometheus) DeleteOrderCreds(orderID uuid.UUID, isSigned bool) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "DeleteOrderCreds", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.DeleteOrderCreds(orderID, isSigned)
}

// GetIssuer implements Datastore
func (_d DatastoreWithPrometheus) GetIssuer(merchantID string) (ip1 *Issuer, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuer", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuer(merchantID)
}

// GetIssuerByPublicKey implements Datastore
func (_d DatastoreWithPrometheus) GetIssuerByPublicKey(publicKey string) (ip1 *Issuer, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuerByPublicKey", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuerByPublicKey(publicKey)
}

// GetKeys implements Datastore
func (_d DatastoreWithPrometheus) GetKeys(merchant string, showExpired bool) (kap1 *[]Key, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetKeys", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetKeys(merchant, showExpired)
}

// GetOrder implements Datastore
func (_d DatastoreWithPrometheus) GetOrder(orderID uuid.UUID) (op1 *Order, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetOrder", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetOrder(orderID)
}

// GetOrderCreds implements Datastore
func (_d DatastoreWithPrometheus) GetOrderCreds(orderID uuid.UUID, isSigned bool) (oap1 *[]OrderCreds, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetOrderCreds", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetOrderCreds(orderID, isSigned)
}

// GetOrderCredsByItemID implements Datastore
func (_d DatastoreWithPrometheus) GetOrderCredsByItemID(orderID uuid.UUID, itemID uuid.UUID, isSigned bool) (op1 *OrderCreds, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetOrderCredsByItemID", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetOrderCredsByItemID(orderID, itemID, isSigned)
}

// GetPagedMerchantTransactions implements Datastore
func (_d DatastoreWithPrometheus) GetPagedMerchantTransactions(ctx context.Context, merchantID uuid.UUID, pagination *inputs.Pagination) (tap1 *[]Transaction, i1 int, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetPagedMerchantTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetPagedMerchantTransactions(ctx, merchantID, pagination)
}

// GetSumForTransactions implements Datastore
func (_d DatastoreWithPrometheus) GetSumForTransactions(orderID uuid.UUID) (d1 decimal.Decimal, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetSumForTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetSumForTransactions(orderID)
}

// GetTransaction implements Datastore
func (_d DatastoreWithPrometheus) GetTransaction(externalTransactionID string) (tp1 *Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetTransaction", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetTransaction(externalTransactionID)
}

// GetTransactions implements Datastore
func (_d DatastoreWithPrometheus) GetTransactions(orderID uuid.UUID) (tap1 *[]Transaction, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetTransactions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetTransactions(orderID)
}

// GetUncommittedVotesForUpdate implements Datastore
func (_d DatastoreWithPrometheus) GetUncommittedVotesForUpdate(ctx context.Context) (tp1 *sqlx.Tx, vpa1 []*VoteRecord, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetUncommittedVotesForUpdate", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetUncommittedVotesForUpdate(ctx)
}

// InsertIssuer implements Datastore
func (_d DatastoreWithPrometheus) InsertIssuer(issuer *Issuer) (ip1 *Issuer, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertIssuer", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertIssuer(issuer)
}

// InsertOrderCreds implements Datastore
func (_d DatastoreWithPrometheus) InsertOrderCreds(creds *OrderCreds) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertOrderCreds", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertOrderCreds(creds)
}

// InsertVote implements Datastore
func (_d DatastoreWithPrometheus) InsertVote(ctx context.Context, vr VoteRecord) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertVote", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertVote(ctx, vr)
}

// MarkVoteErrored implements Datastore
func (_d DatastoreWithPrometheus) MarkVoteErrored(ctx context.Context, vr VoteRecord, tx *sqlx.Tx) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "MarkVoteErrored", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.MarkVoteErrored(ctx, vr, tx)
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

// RunNextOrderJob implements Datastore
func (_d DatastoreWithPrometheus) RunNextOrderJob(ctx context.Context, worker OrderWorker) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RunNextOrderJob", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RunNextOrderJob(ctx, worker)
}

// UpdateOrder implements Datastore
func (_d DatastoreWithPrometheus) UpdateOrder(orderID uuid.UUID, status string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "UpdateOrder", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UpdateOrder(orderID, status)
}

// UpdateOrderMetadata implements Datastore
func (_d DatastoreWithPrometheus) UpdateOrderMetadata(orderID uuid.UUID, key string, value string) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "UpdateOrderMetadata", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.UpdateOrderMetadata(orderID, key, value)
}
