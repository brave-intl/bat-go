package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/inputs"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Settlement holds information from settlements queue
type Settlement struct {
	AltCurrency    altcurrency.AltCurrency `json:"altcurrency"`
	Probi          decimal.Decimal         `json:"probi"`
	Fees           decimal.Decimal         `json:"fees"`
	Fee            decimal.Decimal         `json:"fee"`
	Commission     decimal.Decimal         `json:"commission"`
	Amount         decimal.Decimal         `json:"amount"` // amount in settlement currency
	Currency       string                  `json:"currency"`
	Owner          string                  `json:"owner"`
	Channel        Channel                 `json:"publisher"`
	Hash           string                  `json:"hash"`
	Type           string                  `json:"type"`
	SettlementID   string                  `json:"settlementId"`
	DocumentID     string                  `json:"documentId"`
	Address        string                  `json:"address"`
	ExecutedAt     string                  `json:"executedAt"`
	WalletProvider string                  `json:"walletProvider"`
}

// GenerateID generates an id from the settlement metadata
func (settlement *Settlement) GenerateID(category string) string {
	return uuid.NewV5(
		TransactionNS[category],
		settlement.SettlementID+settlement.Channel.Normalize().String(),
	).String()
}

// ToContributionTransactions creates a set of contribution transactions for settlements
func (settlement *Settlement) ToContributionTransactions() []Transaction {
	createdAt := settlement.GetCreatedAt()
	month := createdAt.Month().String()[:3]
	normalizedChannel := settlement.Channel.Normalize()
	return []Transaction{{
		ID:              settlement.GenerateID("settlement_from_channel"),
		CreatedAt:       createdAt.Add(time.Second * -2),
		Description:     fmt.Sprintf("contributions through %s", month),
		TransactionType: "contribution",
		DocumentID:      settlement.Hash,
		FromAccount:     normalizedChannel.String(),
		FromAccountType: "channel",
		ToAccount:       settlement.Owner,
		ToAccountType:   "owner",
		Amount:          altcurrency.BAT.FromProbi(settlement.Probi.Add(settlement.Fees)),
		Channel:         &normalizedChannel,
	}, {
		ID:              settlement.GenerateID("settlement_fees"),
		CreatedAt:       createdAt.Add(time.Second * -1),
		Description:     "settlement fees",
		TransactionType: "fees",
		DocumentID:      settlement.Hash,
		FromAccount:     settlement.Owner,
		FromAccountType: "owner",
		ToAccount:       "fees-account",
		ToAccountType:   "internal",
		Amount:          altcurrency.BAT.FromProbi(settlement.Fees),
		Channel:         &normalizedChannel,
	}}
}

// GetCreatedAt gets the time the message was created at
func (settlement *Settlement) GetCreatedAt() time.Time {
	createdAt, _ := time.Parse(time.RFC3339, settlement.ExecutedAt)
	return createdAt
}

// GetToAccountType gets the account type
func (settlement *Settlement) GetToAccountType() string {
	toAccountType := "uphold"
	if settlement.WalletProvider != "" {
		toAccountType = settlement.WalletProvider
	}
	return toAccountType
}

// ToManualTransaction creates a manual transaction
func (settlement *Settlement) ToManualTransaction() Transaction {
	return Transaction{
		ID:              settlement.GenerateID(settlement.Type),
		CreatedAt:       settlement.GetCreatedAt(), // do we want to change this?
		Description:     "handshake agreement with business development",
		TransactionType: settlement.Type,
		DocumentID:      settlement.DocumentID,
		FromAccount:     SettlementAddress,
		FromAccountType: settlement.GetToAccountType(),
		ToAccount:       settlement.Owner,
		ToAccountType:   "owner",
		Amount:          altcurrency.BAT.FromProbi(settlement.Probi),
	}
}

// ToSettlementTransaction creates a settlement transaction
func (settlement *Settlement) ToSettlementTransaction() Transaction {
	normalizedChannel := settlement.Channel.Normalize()
	settlementType := settlement.Type
	transactionType := settlementType + "_settlement"
	documentID := settlement.DocumentID
	if settlement.Type != "manual" {
		documentID = settlement.Hash
	}
	return Transaction{
		ID:                 settlement.GenerateID(transactionType),
		CreatedAt:          settlement.GetCreatedAt(),
		Description:        fmt.Sprintf("payout for %s", settlement.Type),
		TransactionType:    transactionType,
		FromAccount:        settlement.Owner,
		DocumentID:         documentID,
		FromAccountType:    "owner",
		ToAccount:          settlement.Address,
		ToAccountType:      settlement.GetToAccountType(), // needs to be updated
		Amount:             altcurrency.BAT.FromProbi(settlement.Probi),
		SettlementCurrency: &settlement.Currency,
		SettlementAmount:   &settlement.Amount,
		Channel:            &normalizedChannel,
	}
}

// ToTxs converts a settlement to the appropriate transactions
func (settlement *Settlement) ToTxs() []Transaction {
	txs := []Transaction{}
	if settlement.Type == "contribution" {
		txs = append(txs, settlement.ToContributionTransactions()...)
	} else if settlement.Type == "manual" {
		txs = append(txs, settlement.ToManualTransaction())
	}
	return append(txs, settlement.ToSettlementTransaction())
}

// Ignore allows us to savely ignore a message if it is malformed
func (settlement *Settlement) Ignore() bool {
	props := settlement.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(settlement.Probi).GreaterThan(largeBAT) ||
		altcurrency.BAT.FromProbi(settlement.Fees).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the message it has everything it needs to be inserted correctly
func (settlement *Settlement) Valid() error {
	// non zero and no decimals allowed
	errs := []error{}
	if !settlement.Probi.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("probi is not greater than zero"))
	}
	if !settlement.Probi.Equal(settlement.Probi.Truncate(0)) {
		errs = append(errs, errors.New("probi is not an int"))
	}
	if !settlement.Fees.GreaterThanOrEqual(decimal.Zero) {
		errs = append(errs, errors.New("fees are negative"))
	}
	if settlement.Owner == "" {
		errs = append(errs, errors.New("owner not set"))
	}
	if settlement.Channel.String() == "" {
		errs = append(errs, errors.New("channel not set"))
	}
	if !settlement.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("amount is not greater than zero"))
	}
	if settlement.Currency == "" {
		errs = append(errs, errors.New("currency is not set"))
	}
	if settlement.Type == "" {
		errs = append(errs, errors.New("transaction type is not set"))
	}
	if settlement.Address == "" {
		errs = append(errs, errors.New("address is not set"))
	}
	if settlement.DocumentID == "" {
		errs = append(errs, errors.New("document id is not set"))
	}
	if settlement.SettlementID == "" {
		errs = append(errs, errors.New("settlement id is not set"))
	}
	if len(errs) != 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}

// ToNative - convert to `native` map
func (settlement *Settlement) ToNative() map[string]interface{} {
	return map[string]interface{}{
		"altcurrency":    settlement.AltCurrency.String(),
		"probi":          settlement.Probi.String(),
		"fees":           settlement.Fees.String(),
		"fee":            settlement.Fee.String(),
		"commission":     settlement.Commission.String(),
		"amount":         settlement.Amount.String(),
		"currency":       settlement.Currency,
		"owner":          settlement.Owner,
		"publisher":      settlement.Channel.String(),
		"hash":           settlement.Hash,
		"type":           settlement.Type,
		"settlementId":   settlement.SettlementID,
		"documentId":     settlement.DocumentID,
		"address":        settlement.Address,
		"executedAt":     settlement.ExecutedAt,
		"walletProvider": settlement.WalletProvider,
	}
}

// SettlementStat holds settlement stats
type SettlementStat struct {
	Amount inputs.Decimal `json:"amount" db:"amount"`
}

// SettlementStatOptions holds options for scaling up and down settlement stats
type SettlementStatOptions struct {
	Type     string
	Start    inputs.Time
	Until    inputs.Time
	Currency *string
}

// NewSettlementStatOptions creates a bundle of settlement stat options
func NewSettlementStatOptions(
	t, currencyInput, layout, start, until string,
) (*SettlementStatOptions, error) {
	startTime, untilTime, err := parseTimes(layout, start, until)
	if err != nil {
		return nil, err
	}
	var currency *string
	if currencyInput != "" {
		currency = &currencyInput
	}
	return &SettlementStatOptions{
		Type:     t,
		Start:    *startTime,
		Until:    *untilTime,
		Currency: currency,
	}, nil
}

// GrantStat holds Grant stats
type GrantStat struct {
	Amount inputs.Decimal `json:"amount" db:"amount"`
	Count  inputs.Decimal `json:"count" db:"count"`
}

// GrantStatOptions holds options for scaling up and down grant stats
type GrantStatOptions struct {
	Type  string
	Start inputs.Time
	Until inputs.Time
}

// NewGrantStatOptions creates a new grant stat options object
func NewGrantStatOptions(
	t, layout, start, until string,
) (*GrantStatOptions, error) {
	startTime, untilTime, err := parseTimes(layout, start, until)
	if err != nil {
		return nil, err
	}
	if !ValidGrantStatTypes[t] {
		return nil, fmt.Errorf("unable to check grant stats for type %s", t)
	}
	return &GrantStatOptions{
		Type:  t,
		Start: *startTime,
		Until: *untilTime,
	}, nil
}

func parseTimes(
	layout, start, until string,
) (*inputs.Time, *inputs.Time, error) {
	var (
		startTime = inputs.NewTime(layout)
		untilTime = inputs.NewTime(layout)
		ctx       = context.Background()
	)
	if err := startTime.Decode(ctx, []byte(start)); err != nil {
		return nil, nil, err
	}
	if err := untilTime.Decode(ctx, []byte(until)); err != nil {
		untilSubMonth := startTime.Time().AddDate(0, -1, 0)
		untilTime.SetTime(untilSubMonth)
	}
	return startTime, untilTime, nil
}
