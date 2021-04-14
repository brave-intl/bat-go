package models

import (
	"errors"
	"time"

	stringutils "github.com/brave-intl/bat-go/utils/string"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	// SettlementAddress is the address where settlements originate
	SettlementAddress = uphold.SettlementDestination
	largeBAT          = decimal.NewFromFloat(1e9)
	// ErrConvertableFailedValidation when a transaction object fails its validation
	ErrConvertableFailedValidation = errors.New("convertable failed validation")
	// TransactionNS hold a hash of the namespaces for transactions
	TransactionNS = map[string]uuid.UUID{
		"ad":                      uuid.FromStringOrNil("2ca02950-084f-475f-bac3-42a3c99dec95"),
		"contribution":            uuid.FromStringOrNil("be90c1a8-20a3-4f32-be29-ed3329ca8630"),
		"contribution_settlement": uuid.FromStringOrNil("4208cdfc-26f3-44a2-9f9d-1f6657001706"),
		"manual":                  uuid.FromStringOrNil("734a27cd-0834-49a5-8d4c-77da38cdfb22"),
		"manual_settlement":       uuid.FromStringOrNil("a7cb6b9e-b0b4-4c40-85bf-27a0172d4353"),
		"referral":                uuid.FromStringOrNil("3d3e7966-87c3-44ed-84c3-252458f99536"),
		"referral_settlement":     uuid.FromStringOrNil("7fda9071-4f0d-4fe6-b3ac-b1c484d5601a"),
		"settlement_from_channel": uuid.FromStringOrNil("eb296f6d-ab2a-489f-bc75-a34f1ff70acb"),
		"settlement_fees":         uuid.FromStringOrNil("1d295e60-e511-41f5-8ae0-46b6b5d33333"),
		"user_deposit":            uuid.FromStringOrNil("f7a8b983-2383-48f2-9e4f-717f6fe3225d"),
		"votes":                   uuid.FromStringOrNil("f0ca8ff9-8399-493a-b2c2-6d4a49e5223a"),
	}
	// SettlementTypes holds type values that are considered settlement types
	SettlementTypes = map[string]bool{
		"contribution_settlement": true,
		"referral_settlement":     true,
	}
	// TransactionColumns columns for transaction queries
	TransactionColumns = stringutils.CollectTags(&Transaction{})
	// SurveyorColumns columns for surveyor group queries
	SurveyorColumns = stringutils.CollectTags(&Surveyor{})
	// TimestampColumns holds the columns that are auto generated and have no need to set
	TimestampColumns = []string{"created_at", "updated_at"}
	// BallotColumns holds columns for ballots
	BallotColumns = stringutils.CollectTags(&Ballot{})
	// ValidGrantStatTypes holds a hash of valid stat types for grants
	ValidGrantStatTypes = map[string]bool{
		"ads": true,
	}
)

// ConvertableTransaction allows a struct to be converted into a transaction
type ConvertableTransaction interface {
	ToTxs() []Transaction
	Valid() error
	Ignore() bool
}

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
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
