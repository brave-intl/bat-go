package custodian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

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
	idempotencyKey *uuid.UUID              `json:"idempotencyKey,omitempty"`
	amount         decimal.Decimal         `json:"amount,omitempty"`
	currency       altcurrency.AltCurrency `json:"currency,omitempty"`
	destination    *uuid.UUID              `json:"destination,omitempty"`
	source         *uuid.UUID              `json:"source,omitempty"`
}

// GetItempotencyKey - implement transaction
func (t *transaction) GetIdempotencyKey(context.Context) (fmt.Stringer, error) {
	return t.idempotencyKey, nil
}

// GetDestination - implement transaction
func (t *transaction) GetDestination(context.Context) (fmt.Stringer, error) {
	return t.destination, nil
}

// GetSource - implement transaction
func (t *transaction) GetSource(context.Context) (fmt.Stringer, error) {
	return t.source, nil
}

// GetAmount - implement transaction
func (t *transaction) GetAmount(context.Context) (decimal.Decimal, error) {
	return t.amount, nil
}

// GetCurrency - implement transaction
func (t *transaction) GetCurrency(context.Context) (altcurrency.AltCurrency, error) {
	return t.currency, nil
}

// NewTransaction - create a new transaction
func NewTransaction(ctx context.Context, idempotencyKey, destination, source *uuid.UUID, currency altcurrency.AltCurrency, amount decimal.Decimal) (Transaction, error) {
	return &transaction{
		idempotencyKey: idempotencyKey,
		destination:    destination,
		source:         source,
		currency:       currency,
		amount:         amount,
	}, nil
}

// Custodian - interface defining what a custodian is
type Custodian interface {
	SubmitTransactions(context.Context, ...Transaction) error
	GetTransactionsStatus(context.Context, ...Transaction) (map[string]TransactionStatus, error)
}

// CustodianConfig - configurations for each custodian
type CustodianConfig struct {
	provider string `valid:"in(uphold,gemini,bitflyer)"`
	config   map[appctx.CTXKey]interface{}
}

// String - implement stringer
func (cc *CustodianConfig) String() string {
	// convert to json
	b, err := json.Marshal(cc)
	if err != nil {
		return fmt.Sprintf("failed to marshal CustodianConfig: %s", err.Error())
	}
	return string(b)
}

var (
	ErrConfigValidation = errors.New("failed to validate custodian configuration")
)

// New - create new custodian
func New(ctx context.Context, conf CustodianConfig) (Custodian, error) {
	logger := loggingutils.Logger(ctx, "custodian.New").With().Str("conf", conf.String()).Logger()
	// validate the configuration
	logger.Debug().Msg("about to validate custodian config")
	_, err := govalidator.ValidateStruct(conf)
	if err != nil {
		return nil, loggingutils.LogAndError(
			&logger, ErrConfigValidation.Error(), fmt.Errorf("%w: %s", ErrConfigValidation, err.Error()))
	}
	switch conf.provider {
	case "uphold":
		logger.Debug().Msg("creating uphold custodian")
		return newUpholdCustodian(ctx, conf)
	case "gemini":
		logger.Debug().Msg("creating gemini custodian")
		return newGeminiCustodian(ctx, conf)
	case "bitflyer":
		logger.Debug().Msg("creating bitflyer custodian")
		return newBitflyerCustodian(ctx, conf)
	default:
		msg := "invalid provider"
		return nil, loggingutils.LogAndError(
			&logger, msg, fmt.Errorf(
				"%w: invalid provider \"%s\" not in (uphold,gemini,bitflyer)",
				ErrConfigValidation, conf.provider))
	}
}
