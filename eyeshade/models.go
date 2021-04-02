package eyeshade

import (
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/inputs"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	UUIDNamespaces = map[string]uuid.UUID{
		"contribution":            uuid.FromStringOrNil("be90c1a8-20a3-4f32-be29-ed3329ca8630"),
		"contribution_settlement": uuid.FromStringOrNil("4208cdfc-26f3-44a2-9f9d-1f6657001706"),
		"manual":                  uuid.FromStringOrNil("734a27cd-0834-49a5-8d4c-77da38cdfb22"),
		"manual_settlement":       uuid.FromStringOrNil("a7cb6b9e-b0b4-4c40-85bf-27a0172d4353"),
		"referral":                uuid.FromStringOrNil("3d3e7966-87c3-44ed-84c3-252458f99536"),
		"referral_settlement":     uuid.FromStringOrNil("7fda9071-4f0d-4fe6-b3ac-b1c484d5601a"),
	}
	transactionColumns = []string{
		"id",
		"created_at",
		"description",
		"transaction_type",
		"document_id",
		"from_account",
		"from_account_type",
		"to_account",
		"to_account_type",
		"amount",
		"settlement_currency",
		"settlement_amount",
		"channel",
	}
)

// ConvertableTransaction allows a struct to be converted into a transaction
type ConvertableTransaction interface {
	ToTxs() (*[]Transaction, error)
	Validate() bool
}

// Settlement holds information from settlements queue
type Settlement struct {
	AltCurrency  altcurrency.AltCurrency
	Probi        decimal.Decimal
	Fees         decimal.Decimal
	Amount       decimal.Decimal // amount in settlement currency
	Currency     string
	Owner        string
	Channel      string
	ID           string
	Type         string
	SettlementID string
	DocumentID   string
	ExecutedAt   *time.Time
	Address      string
}

func (settlement *Settlement) ToTxs() (*[]Transaction, error) {
	txs := []Transaction{}
	if settlement.AltCurrency != altcurrency.BAT {

	}
	return &txs, nil
}

func (settlement *Settlement) Validate() bool {
	return true
}

// Referral holds information from referral queue
type Referral struct {
	AltCurrency       altcurrency.AltCurrency
	Probi             decimal.Decimal
	Channel           string
	TransactionID     string
	SettlementAddress string
	Owner             string
	FirstID           time.Time
}

func (referral Referral) ToTxs() (*[]Transaction, error) {
	txs := []Transaction{}
	return &txs, nil
}

func (referral Referral) Validate() bool {
	return true
}

// Votes holds information from votes freezing
type Votes struct {
	Amount            decimal.Decimal
	Fees              decimal.Decimal
	Channel           string
	SurveyorID        string
	SurveyorCreatedAt time.Time
	SettlementAddress string
}

func (votes Votes) ToTxs() (*[]Transaction, error) {
	txs := []Transaction{}
	return &txs, nil
}

func (votes Votes) Validate() bool {
	return true
}

// UserDeposit holds information from user deposits
type UserDeposit struct {
	ID        string
	Amount    decimal.Decimal
	Chain     string
	CardID    string
	CreatedAt time.Time
	Address   string
}

func (userDeposit UserDeposit) ToTxs() (*[]Transaction, error) {
	txs := []Transaction{}
	return &txs, nil
}

func (userDeposit UserDeposit) Validate() bool {
	return true
}

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   string          `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   string          `json:"channel" db:"channel"`
	Paid      decimal.Decimal `json:"paid" db:"paid"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountEarningsOptions receives all options pertaining to account earnings calculations
type AccountEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
}

// AccountSettlementEarningsOptions receives all options pertaining to account settlement earnings calculations
type AccountSettlementEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
	StartDate *time.Time
	UntilDate *time.Time
}

// Balance holds information about an account id's balance
type Balance struct {
	AccountID string          `json:"account_id" db:"account_id"`
	Type      string          `json:"account_type" db:"account_type"`
	Balance   decimal.Decimal `json:"balance" db:"balance"`
}

// PendingTransaction holds information about an account id's pending transactions
type PendingTransaction struct {
	Channel string          `db:"channel"`
	Balance decimal.Decimal `db:"balance"`
}

// Transaction holds info about a single transaction from the database
type Transaction struct {
	ID                 string           `db:"id"`
	CreatedAt          time.Time        `db:"created_at"`
	Description        string           `db:"description"`
	TransactionType    string           `db:"transaction_type"`
	DocumentID         string           `db:"document_id"`
	FromAccount        string           `db:"from_account"`
	FromAccountType    string           `db:"from_account_type"`
	ToAccount          string           `db:"to_account"`
	ToAccountType      string           `db:"to_account_type"`
	Amount             decimal.Decimal  `db:"amount"`
	SettlementCurrency *string          `db:"settlement_currency"`
	SettlementAmount   *decimal.Decimal `db:"settlement_amount"`
	Channel            string           `db:"channel"`
}

// Backfill converts a transaction from the database to a backfill transaction
func (tx Transaction) BackfillForCreators(account string) CreatorsTransaction {
	amount := tx.Amount
	if tx.FromAccount == account {
		amount = amount.Neg()
	}
	var settlementDestinationType *string
	var settlementDestination *string
	if settlementTypes[tx.TransactionType] {
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
	Channel                   string         `json:"channel"`
	Amount                    inputs.Decimal `json:"amount"`
	TransactionType           string         `json:"transaction_type"`
	SettlementCurrency        *string        `json:"settlement_currency,omitempty"`
	SettlementAmount          inputs.Decimal `json:"settlement_amount,omitempty"`
	SettlementDestinationType *string        `json:"settlement_destination_type,omitempty"`
	SettlementDestination     *string        `json:"settlement_destination,omitempty"`
}
