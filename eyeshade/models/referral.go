package models

import (
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Referral holds information from referral queue
type Referral struct {
	AltCurrency   altcurrency.AltCurrency
	Probi         decimal.Decimal
	Channel       Channel
	TransactionID string
	Owner         string
	FirstID       time.Time
}

// GenerateID creates a uuidv5 generated identifier from the referral data
func (referral *Referral) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["referral"],
		referral.TransactionID,
	).String()
}

// ToTxs converts a referral to the appropriate transactions
func (referral *Referral) ToTxs() *[]Transaction {
	month := referral.FirstID.Month().String()[:3]
	normalizedChannel := referral.Channel.Normalize()
	return &[]Transaction{{
		ID:              referral.GenerateID(),
		CreatedAt:       referral.FirstID,
		Description:     fmt.Sprintf("referrals through %s", month),
		TransactionType: "referral",
		DocumentID:      referral.TransactionID,
		FromAccount:     SettlementAddress,
		FromAccountType: "uphold",
		ToAccount:       referral.Owner,
		ToAccountType:   "owner",
		Amount:          altcurrency.BAT.FromProbi(referral.Probi),
		Channel:         &normalizedChannel,
	}}
}

// Ignore allows us to savely ignore the transaction if necessary
func (referral *Referral) Ignore() bool {
	props := referral.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(referral.Probi).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the transaction has the necessary data to be inserted correctly
func (referral *Referral) Valid() bool {
	return referral.AltCurrency.IsValid() &&
		referral.Probi.GreaterThan(decimal.Zero) &&
		referral.Probi.Equal(referral.Probi.Truncate(0)) && // no decimals allowed
		referral.Channel != "" &&
		referral.Owner != "" &&
		!referral.FirstID.IsZero()
}
