package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
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
func (referral *Referral) ToTxs() []Transaction {
	month := referral.FirstID.Month().String()[:3]
	normalizedChannel := referral.Channel.Normalize()
	return []Transaction{{
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
func (referral *Referral) Valid() error {
	errs := []error{}
	if !referral.AltCurrency.IsValid() {
		errs = append(errs, errors.New("altcurrency is not valid"))
	}
	if !referral.Probi.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("probi is not greater than zero"))
	}
	if !referral.Probi.Equal(referral.Probi.Truncate(0)) {
		errs = append(errs, errors.New("probi is not an int"))
	}
	if referral.Channel.String() == "" {
		errs = append(errs, errors.New("channel is not set"))
	}
	if referral.Owner == "" {
		errs = append(errs, errors.New("owner is not set"))
	}
	if referral.FirstID.IsZero() {
		errs = append(errs, errors.New("the first id from the referral is zero"))
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}
