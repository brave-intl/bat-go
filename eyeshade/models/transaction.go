package models

import (
	"time"

	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/shopspring/decimal"
)

// PendingTransaction holds information about an account id's pending transactions
type PendingTransaction struct {
	Channel Channel         `db:"channel"`
	Balance decimal.Decimal `db:"balance"`
}

// Transaction holds info about a single transaction from the database
type Transaction struct {
	ID                 string           `json:"id" db:"id"`
	CreatedAt          time.Time        `json:"createdAt" db:"created_at"`
	Description        string           `json:"description" db:"description"`
	TransactionType    string           `json:"transactionType" db:"transaction_type"`
	DocumentID         string           `json:"documentId" db:"document_id"`
	FromAccount        string           `json:"fromAccount" db:"from_account"`
	FromAccountType    string           `json:"fromAccountType" db:"from_account_type"`
	ToAccount          string           `json:"toAccount" db:"to_account"`
	ToAccountType      string           `json:"toAccountType" db:"to_account_type"`
	Amount             decimal.Decimal  `json:"amount" db:"amount"`
	SettlementCurrency *string          `json:"settlementCurrency" db:"settlement_currency"`
	SettlementAmount   *decimal.Decimal `json:"settlementAmount" db:"settlement_amount"`
	Channel            *Channel         `json:"channel" db:"channel"`
}

// BackfillForCreators converts a transaction from the database to a backfill transaction
func (tx *Transaction) BackfillForCreators(account string) CreatorsTransaction {
	amount := tx.Amount
	if tx.FromAccount == account {
		amount = amount.Neg()
	}
	var settlementDestinationType *string
	var settlementDestination *string
	if IsSettlementTypeSuffixPresent(tx.TransactionType) {
		if tx.ToAccountType != "" {
			settlementDestinationType = &tx.ToAccountType
		}
		if tx.ToAccount != "" {
			settlementDestination = &tx.ToAccount
		}
	}
	inputAmount := inputs.NewDecimal(&amount)
	inputSettlementAmount := inputs.NewDecimal(tx.SettlementAmount)
	return CreatorsTransaction{
		Amount:                    inputAmount,
		Channel:                   tx.Channel,
		CreatedAt:                 tx.CreatedAt,
		Description:               tx.Description,
		SettlementCurrency:        tx.SettlementCurrency,
		SettlementAmount:          inputSettlementAmount,
		TransactionType:           tx.TransactionType,
		SettlementDestinationType: settlementDestinationType,
		SettlementDestination:     settlementDestination,
	}
}

// CreatorsTransaction holds a backfilled version of the transaction
type CreatorsTransaction struct {
	CreatedAt                 time.Time      `json:"created_at"`
	Description               string         `json:"description"`
	Channel                   *Channel       `json:"channel"`
	Amount                    inputs.Decimal `json:"amount"`
	TransactionType           string         `json:"transaction_type"`
	SettlementCurrency        *string        `json:"settlement_currency,omitempty"`
	SettlementAmount          inputs.Decimal `json:"settlement_amount,omitempty"`
	SettlementDestinationType *string        `json:"settlement_destination_type,omitempty"`
	SettlementDestination     *string        `json:"settlement_destination,omitempty"`
}
