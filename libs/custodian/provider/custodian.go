package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	appctx "github.com/brave-intl/bat-go/libs/context"
	loggingutils "github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	// Uphold - custodian identifier for uphold
	Uphold = "uphold"
	// Gemini - custodian identifier for gemini
	Gemini = "gemini"
	// Bitflyer - custodian identifier for bitflyer
	Bitflyer = "bitflyer"
)

// TransactionStatus - stringer repr of transaction status
type TransactionStatus fmt.Stringer

// Transaction - interface defining what a transaction is
type Transaction interface {
	GetIdempotencyKey(context.Context) (fmt.Stringer, error)
	GetAmount(context.Context) (decimal.Decimal, error)
	GetCurrency(context.Context) (altcurrency.AltCurrency, error)
	GetDestination(context.Context) (fmt.Stringer, error)
	GetSource(context.Context) (fmt.Stringer, error)
}

// implementation of Transaction
type transaction struct {
	IdempotencyKey *uuid.UUID              `json:"idempotencyKey,omitempty"`
	Amount         decimal.Decimal         `json:"amount,omitempty"`
	Currency       altcurrency.AltCurrency `json:"currency,omitempty"`
	Destination    *uuid.UUID              `json:"destination,omitempty"`
	Source         *uuid.UUID              `json:"source,omitempty"`
}

// GetItempotencyKey - implement transaction
func (t *transaction) GetIdempotencyKey(context.Context) (fmt.Stringer, error) {
	return t.IdempotencyKey, nil
}

// GetDestination - implement transaction
func (t *transaction) GetDestination(context.Context) (fmt.Stringer, error) {
	return t.Destination, nil
}

// GetSource - implement transaction
func (t *transaction) GetSource(context.Context) (fmt.Stringer, error) {
	return t.Source, nil
}

// GetAmount - implement transaction
func (t *transaction) GetAmount(context.Context) (decimal.Decimal, error) {
	return t.Amount, nil
}

// GetCurrency - implement transaction
func (t *transaction) GetCurrency(context.Context) (altcurrency.AltCurrency, error) {
	return t.Currency, nil
}

// NewTransaction - create a new transaction
func NewTransaction(ctx context.Context, idempotencyKey, destination, source *uuid.UUID, currency altcurrency.AltCurrency, amount decimal.Decimal) (Transaction, error) {
	return &transaction{
		IdempotencyKey: idempotencyKey,
		Destination:    destination,
		Source:         source,
		Currency:       currency,
		Amount:         amount,
	}, nil
}

// Custodian - interface defining what a custodian is
type Custodian interface {
	SubmitTransactions(context.Context, ...Transaction) error
	GetTransactionsStatus(context.Context, ...Transaction) (map[string]TransactionStatus, error)
}

// Config - configurations for each custodian
type Config struct {
	Provider string
	Config   map[appctx.CTXKey]interface{}
}

// String - implement stringer
func (cc *Config) String() string {
	// convert to json
	b, err := json.Marshal(cc)
	if err != nil {
		return fmt.Sprintf("failed to marshal Config: %s", err.Error())
	}
	return string(b)
}

var (
	// ErrConfigValidation - error for validation config
	ErrConfigValidation = errors.New("failed to validate custodian configuration")
)

// New - create new custodian
func New(ctx context.Context, conf Config) (Custodian, error) {
	logger := loggingutils.Logger(ctx, "custodian.New").With().Str("conf", conf.String()).Logger()
	// validate the configuration
	logger.Debug().Msg("about to validate custodian config")
	_, err := govalidator.ValidateStruct(conf)
	if err != nil {
		return nil, loggingutils.LogAndError(
			&logger, ErrConfigValidation.Error(), fmt.Errorf("%w: %s", ErrConfigValidation, err.Error()))
	}
	switch conf.Provider {
	case Uphold:
		logger.Debug().Msg("creating uphold custodian")
		return newUpholdCustodian(ctx, conf)
	case Gemini:
		logger.Debug().Msg("creating gemini custodian")
		return newGeminiCustodian(ctx, conf)
	case Bitflyer:
		logger.Debug().Msg("creating bitflyer custodian")
		return newBitflyerCustodian(ctx, conf)
	default:
		msg := "invalid provider"
		return nil, loggingutils.LogAndError(
			&logger, msg, fmt.Errorf(
				"%w: invalid provider \"%s\" not in (uphold,gemini,bitflyer)",
				ErrConfigValidation, conf.Provider))
	}
}
