package promotion

// DO NOT EDIT!
// This code is generated with http://github.com/hexdigest/gowrap tool
// using https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus template

//go:generate gowrap gen -p github.com/brave-intl/bat-go/promotion -i Datastore -t https://raw.githubusercontent.com/hexdigest/gowrap/1741ed8de90dd8c90b4939df7f3a500ac9922b1b/templates/prometheus -o instrumented_datastore.go

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/golang-migrate/migrate/v4"
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
		Name:       "promotion_datastore_duration_seconds",
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

// ActivatePromotion implements Datastore
func (_d DatastoreWithPrometheus) ActivatePromotion(promotion *Promotion) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "ActivatePromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ActivatePromotion(promotion)
}

// ClaimForWallet implements Datastore
func (_d DatastoreWithPrometheus) ClaimForWallet(promotion *Promotion, issuer *Issuer, wallet *wallet.Info, blindedCreds jsonutils.JSONStringArray) (cp1 *Claim, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "ClaimForWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.ClaimForWallet(promotion, issuer, wallet, blindedCreds)
}

// CreateClaim implements Datastore
func (_d DatastoreWithPrometheus) CreateClaim(promotionID uuid.UUID, walletID string, value decimal.Decimal, bonus decimal.Decimal) (cp1 *Claim, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CreateClaim", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreateClaim(promotionID, walletID, value, bonus)
}

// CreatePromotion implements Datastore
func (_d DatastoreWithPrometheus) CreatePromotion(promotionType string, numGrants int, value decimal.Decimal, platform string) (pp1 *Promotion, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "CreatePromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.CreatePromotion(promotionType, numGrants, value, platform)
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

// DeactivatePromotion implements Datastore
func (_d DatastoreWithPrometheus) DeactivatePromotion(promotion *Promotion) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "DeactivatePromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.DeactivatePromotion(promotion)
}

// DrainClaim implements Datastore
func (_d DatastoreWithPrometheus) DrainClaim(claim *Claim, credentials []cbr.CredentialRedemption, wallet *wallet.Info, total decimal.Decimal) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "DrainClaim", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.DrainClaim(claim, credentials, wallet, total)
}

// GetAvailablePromotions implements Datastore
func (_d DatastoreWithPrometheus) GetAvailablePromotions(platform string, legacy bool) (pa1 []Promotion, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAvailablePromotions", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAvailablePromotions(platform, legacy)
}

// GetAvailablePromotionsForWallet implements Datastore
func (_d DatastoreWithPrometheus) GetAvailablePromotionsForWallet(wallet *wallet.Info, platform string, legacy bool) (pa1 []Promotion, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetAvailablePromotionsForWallet", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetAvailablePromotionsForWallet(wallet, platform, legacy)
}

// GetClaimByWalletAndPromotion implements Datastore
func (_d DatastoreWithPrometheus) GetClaimByWalletAndPromotion(wallet *wallet.Info, promotionID *Promotion) (cp1 *Claim, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetClaimByWalletAndPromotion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetClaimByWalletAndPromotion(wallet, promotionID)
}

// GetClaimCreds implements Datastore
func (_d DatastoreWithPrometheus) GetClaimCreds(claimID uuid.UUID) (cp1 *ClaimCreds, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetClaimCreds", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetClaimCreds(claimID)
}

// GetClaimSummary implements Datastore
func (_d DatastoreWithPrometheus) GetClaimSummary(walletID uuid.UUID, grantType string) (cp1 *ClaimSummary, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetClaimSummary", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetClaimSummary(walletID, grantType)
}

// GetIssuer implements Datastore
func (_d DatastoreWithPrometheus) GetIssuer(promotionID uuid.UUID, cohort string) (ip1 *Issuer, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetIssuer", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetIssuer(promotionID, cohort)
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

// GetPreClaim implements Datastore
func (_d DatastoreWithPrometheus) GetPreClaim(promotionID uuid.UUID, walletID string) (cp1 *Claim, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "GetPreClaim", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.GetPreClaim(promotionID, walletID)
}

// GetPromotion implements Datastore
func (_d DatastoreWithPrometheus) GetPromotion(promotionID uuid.UUID) (pp1 *Promotion, err error) {
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

// InsertClobberedClaims implements Datastore
func (_d DatastoreWithPrometheus) InsertClobberedClaims(ctx context.Context, ids []uuid.UUID, version int) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertClobberedClaims", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertClobberedClaims(ctx, ids, version)
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

// InsertSuggestion implements Datastore
func (_d DatastoreWithPrometheus) InsertSuggestion(credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "InsertSuggestion", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.InsertSuggestion(credentials, suggestionText, suggestion)
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

// RunNextClaimJob implements Datastore
func (_d DatastoreWithPrometheus) RunNextClaimJob(ctx context.Context, worker ClaimWorker) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RunNextClaimJob", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RunNextClaimJob(ctx, worker)
}

// RunNextDrainJob implements Datastore
func (_d DatastoreWithPrometheus) RunNextDrainJob(ctx context.Context, worker DrainWorker) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RunNextDrainJob", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RunNextDrainJob(ctx, worker)
}

// RunNextSuggestionJob implements Datastore
func (_d DatastoreWithPrometheus) RunNextSuggestionJob(ctx context.Context, worker SuggestionWorker) (b1 bool, err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "RunNextSuggestionJob", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.RunNextSuggestionJob(ctx, worker)
}

// SaveClaimCreds implements Datastore
func (_d DatastoreWithPrometheus) SaveClaimCreds(claimCreds *ClaimCreds) (err error) {
	_since := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		datastoreDurationSummaryVec.WithLabelValues(_d.instanceName, "SaveClaimCreds", result).Observe(time.Since(_since).Seconds())
	}()
	return _d.base.SaveClaimCreds(claimCreds)
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
