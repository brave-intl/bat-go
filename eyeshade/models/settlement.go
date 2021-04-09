package models

import (
	"context"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
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
	SettlementID   string                  `json:"transactionId"`
	DocumentID     string                  `json:"documentId"`
	Address        string                  `json:"address"`
	ExecutedAt     *time.Time              `json:"executedAt"`
	WalletProvider *string                 `json:"walletProvider"`
}

// CreatedAt computes the created at time of the settlement
func (settlement *Settlement) CreatedAt() time.Time {
	createdAt := settlement.ExecutedAt
	if createdAt != nil {
		return *createdAt
	}
	// parsed, err := time.Parse(time.RFC3339, settlement.Hash)
	// if err == nil {
	// 	return parsed
	// }
	// id := settlement.Hash
	// hexID := hex.EncodeToString([]byte(id))[:8]
	// parsedSec, err := strconv.ParseInt(hexID, 16, 64)
	// if err != nil {
	// 	return time.Unix(parsedSec, 0)
	// }
	return time.Now()
}

// GenerateID generates an id from the settlement metadata
func (settlement *Settlement) GenerateID(category string) string {
	return uuid.NewV5(
		TransactionNS[category],
		settlement.SettlementID+settlement.Channel.Normalize().String(),
	).String()
}

// ToTxs converts a settlement to the appropriate transactions
func (settlement *Settlement) ToTxs() *[]Transaction {
	txs := []Transaction{}
	createdAt := settlement.CreatedAt()
	month := createdAt.Month().String()[:3]
	normalizedChannel := settlement.Channel.Normalize()
	settlementType := settlement.Type
	transactionType := settlementType + "_settlement"
	toAccountType := "uphold"
	if settlement.WalletProvider != nil {
		toAccountType = *settlement.WalletProvider
	}
	if settlementType == "contribution" {
		txs = append(txs, Transaction{
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
		})
		txs = append(txs, Transaction{
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
		})
	} else if settlementType == "manual" {
		txs = append(txs, Transaction{
			ID:              settlement.GenerateID(settlementType),
			CreatedAt:       createdAt, // do we want to change this?
			Description:     "handshake agreement with business development",
			TransactionType: settlementType,
			DocumentID:      settlement.DocumentID,
			FromAccount:     SettlementAddress,
			FromAccountType: toAccountType,
			ToAccount:       settlement.Owner,
			ToAccountType:   "owner",
			Amount:          altcurrency.BAT.FromProbi(settlement.Probi),
		})
	}
	txs = append(txs, Transaction{
		ID:                 settlement.GenerateID(transactionType),
		CreatedAt:          createdAt,
		Description:        fmt.Sprintf("payout for %s", settlement.Type),
		TransactionType:    transactionType,
		FromAccount:        settlement.Owner,
		FromAccountType:    "owner",
		ToAccount:          settlement.Address,
		ToAccountType:      toAccountType, // needs to be updated
		Amount:             altcurrency.BAT.ToProbi(settlement.Probi),
		SettlementCurrency: &settlement.Currency,
		SettlementAmount:   &settlement.Amount,
		Channel:            &normalizedChannel,
	})
	return &txs
}

// Ignore allows us to savely ignore a message if it is malformed
func (settlement *Settlement) Ignore() bool {
	props := settlement.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(settlement.Probi).GreaterThan(largeBAT) ||
		altcurrency.BAT.FromProbi(settlement.Fees).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the message it has everything it needs to be inserted correctly
func (settlement *Settlement) Valid() bool {
	// non zero and no decimals allowed
	// fmt.Println("!settlement.Probi.GreaterThan(decimal.Zero)", !settlement.Probi.GreaterThan(decimal.Zero))
	// fmt.Println("settlement.Probi.Equal(settlement.Probi.Truncate(0))", settlement.Probi.Equal(settlement.Probi.Truncate(0)))
	// fmt.Println("settlement.Fees.GreaterThanOrEqual(decimal.Zero)", settlement.Fees.GreaterThanOrEqual(decimal.Zero))
	// fmt.Println("settlement.Owner != \"\"", settlement.Owner != "")
	// fmt.Println("settlement.Channel != \"\"", settlement.Channel != "")
	// fmt.Println("settlement.Amount.GreaterThan(decimal.Zero)", settlement.Amount.GreaterThan(decimal.Zero))
	// fmt.Println("settlement.Currency != \"\"", settlement.Currency != "")
	// fmt.Println("settlement.Type != \"\"", settlement.Type != "")
	// fmt.Println("settlement.Address != \"\"", settlement.Address != "")
	// fmt.Println("settlement.DocumentID != \"\"", settlement.DocumentID != "")
	// fmt.Println("settlement.SettlementID != \"\"", settlement.SettlementID != "")
	return settlement.Probi.GreaterThan(decimal.Zero) &&
		settlement.Probi.Equal(settlement.Probi.Truncate(0)) && // no decimals
		settlement.Fees.GreaterThanOrEqual(decimal.Zero) &&
		settlement.Owner != "" &&
		settlement.Channel != "" &&
		settlement.Amount.GreaterThan(decimal.Zero) &&
		settlement.Currency != "" &&
		settlement.Type != "" &&
		settlement.Address != "" &&
		settlement.DocumentID != "" &&
		settlement.SettlementID != ""
}

// ToNative - convert to `native` map
func (settlement *Settlement) ToNative() map[string]interface{} {
	var executedAt *string = nil
	if settlement.ExecutedAt != nil {
		executedFormatted := settlement.ExecutedAt.Format(time.RFC3339)
		executedAt = &executedFormatted
	}
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
		"executedAt":     executedAt,
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
