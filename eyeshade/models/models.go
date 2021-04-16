package models

import (
	"errors"

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
	// AltKeys holds keys that do not fit in any one category
	AltKeys = altKeys{
		Votes: "votes",
	}
	// TransactionNS hold a hash of the namespaces for transactions
	TransactionNS = map[string]uuid.UUID{
		TransactionTypes.Ad:                                     uuid.FromStringOrNil("2ca02950-084f-475f-bac3-42a3c99dec95"),
		TransactionTypes.Contribution:                           uuid.FromStringOrNil("be90c1a8-20a3-4f32-be29-ed3329ca8630"),
		SettlementKeys.AddSuffix(TransactionTypes.Contribution): uuid.FromStringOrNil("4208cdfc-26f3-44a2-9f9d-1f6657001706"),
		TransactionTypes.Manual:                                 uuid.FromStringOrNil("734a27cd-0834-49a5-8d4c-77da38cdfb22"),
		SettlementKeys.AddSuffix(TransactionTypes.Manual):       uuid.FromStringOrNil("a7cb6b9e-b0b4-4c40-85bf-27a0172d4353"),
		TransactionTypes.Referral:                               uuid.FromStringOrNil("3d3e7966-87c3-44ed-84c3-252458f99536"),
		SettlementKeys.AddSuffix(TransactionTypes.Referral):     uuid.FromStringOrNil("7fda9071-4f0d-4fe6-b3ac-b1c484d5601a"),
		SettlementKeys.SettlementFromChannel:                    uuid.FromStringOrNil("eb296f6d-ab2a-489f-bc75-a34f1ff70acb"),
		SettlementKeys.SettlementFees:                           uuid.FromStringOrNil("1d295e60-e511-41f5-8ae0-46b6b5d33333"),
		TransactionTypes.UserDeposit:                            uuid.FromStringOrNil("f7a8b983-2383-48f2-9e4f-717f6fe3225d"),
		AltKeys.Votes:                                           uuid.FromStringOrNil("f0ca8ff9-8399-493a-b2c2-6d4a49e5223a"),
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

type altKeys struct {
	Votes string
}
