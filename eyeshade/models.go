package eyeshade

import (
	"errors"
	"regexp"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/inputs"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	providerRE = regexp.MustCompile(`/^([A-Za-z0-9][A-Za-z0-9-]{0,62})#([A-Za-z0-9][A-Za-z0-9-]{0,62}):(([A-Za-z0-9-._~]|%[0-9A-F]{2})+)$/`)
	// ErrConvertableFailedValidation when a transaction object fails its validation
	ErrConvertableFailedValidation = errors.New("convertable failed validation")

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
	ToTxs() *[]Transaction
	Valid() bool
}

// Settlement holds information from settlements queue
type Settlement struct {
	AltCurrency  altcurrency.AltCurrency
	Probi        decimal.Decimal
	Fees         decimal.Decimal
	Amount       decimal.Decimal // amount in settlement currency
	Currency     string
	Owner        string
	Channel      Channel
	ID           string
	Type         string
	SettlementID string
	DocumentID   string
	ExecutedAt   *time.Time
	Address      string
}

func (settlement *Settlement) ToTxs() *[]Transaction {
	return &[]Transaction{}
}

func (settlement *Settlement) Valid() bool {
	// non zero and no decimals allowed
	return !settlement.Probi.GreaterThan(decimal.Zero) &&
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

// Referral holds information from referral queue
type Referral struct {
	AltCurrency       altcurrency.AltCurrency
	Probi             decimal.Decimal
	Channel           Channel
	TransactionID     string
	SettlementAddress string
	Owner             string
	FirstID           time.Time
}

func (referral Referral) ToTxs() *[]Transaction {
	return &[]Transaction{}
}

func (referral Referral) Valid() bool {
	return referral.AltCurrency.IsValid() &&
		referral.Probi.GreaterThan(decimal.Zero) &&
		referral.Probi.Equal(referral.Probi.Truncate(0)) && // no decimals allowed
		referral.Channel != "" &&
		referral.Owner != "" &&
		!referral.FirstID.IsZero()
}

// Votes holds information from votes freezing
type Votes struct {
	Amount            decimal.Decimal
	Fees              decimal.Decimal
	Channel           Channel
	SurveyorID        string
	SurveyorCreatedAt time.Time
	SettlementAddress string
}

func (votes Votes) GenerateID() {
	return uuid.NewV5(
		UUIDNamespaces["contribution"],
		votes.SurveyorID+votes.Channel.Normalize(),
	)
}

func (votes Votes) ToTxs() *[]Transaction {
	return &[]Transaction{
		{
			ID: votes.GenerateID(),
			CreatedAt: votes.CreatedAt,
			Description: fmt.Sprintf("votes from %s", votes.SurveyorID),
			FromAccountType: "uphold",
		}
	}
}

func (votes Votes) Valid() bool {
	return votes.Amount.GreaterThan(decimal.Zero) &&
		votes.SurveyorID != "" &&
		votes.SettlementAddress != "" &&
		votes.Channel != "" &&
		votes.Fees.GreaterThanOrEqual(decimal.Zero)
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

func (userDeposit UserDeposit) ToTxs() *[]Transaction {
	return &[]Transaction{}
}

func (userDeposit UserDeposit) Valid() bool {
	return userDeposit.CardID != "" &&
		!userDeposit.CreatedAt.IsZero() &&
		userDeposit.Amount.GreaterThan(decimal.Zero) &&
		userDeposit.ID != "" &&
		userDeposit.Chain != "" &&
		userDeposit.Address != ""
}

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   Channel          `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   Channel          `json:"channel" db:"channel"`
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
	Channel Channel          `db:"channel"`
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
	Channel            Channel           `db:"channel"`
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
	Channel                   Channel         `json:"channel"`
	Amount                    inputs.Decimal `json:"amount"`
	TransactionType           string         `json:"transaction_type"`
	SettlementCurrency        *string        `json:"settlement_currency,omitempty"`
	SettlementAmount          inputs.Decimal `json:"settlement_amount,omitempty"`
	SettlementDestinationType *string        `json:"settlement_destination_type,omitempty"`
	SettlementDestination     *string        `json:"settlement_destination,omitempty"`
}
