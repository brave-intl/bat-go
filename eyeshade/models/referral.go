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
	TransactionID      string                  `json:"transactionId"`
	Channel            Channel                 `json:"channelId"`
	Owner              string                  `json:"ownerId"`
	FinalizedTimestamp string                  `json:"finalizedTimestamp"`
	ReferralCode       string                  `json:"referralCode"`
	DownloadID         string                  `json:"downloadId"`
	DownloadTimestamp  string                  `json:"downloadTimestamp"`
	CountryGroupID     string                  `json:"countryGroupId"`
	Platform           string                  `json:"platform"`
	Probi              decimal.Decimal         `json:"probi"`
	AltCurrency        altcurrency.AltCurrency `json:"altcurrency"`
}

// GetTransactionID gets the transaction id
func (referral *Referral) GetTransactionID() string {
	txID := referral.TransactionID
	if txID == "" {
		txID = referral.DownloadID
	}
	return txID
}

// GetFinalizedTimestamp gets a time version of the finalized timestamp
func (referral *Referral) GetFinalizedTimestamp() time.Time {
	tsmp, _ := time.Parse(time.RFC3339, referral.FinalizedTimestamp)
	return tsmp
}

// GenerateID creates a uuidv5 generated identifier from the referral data
func (referral *Referral) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["referral"],
		referral.GetTransactionID(),
	).String()
}

// ToTxs converts a referral to the appropriate transactions
func (referral *Referral) ToTxs() []Transaction {
	owner := referral.Owner
	if owner == "removed" {
		return []Transaction{}
	}
	prefix := "publishers#uuid:"
	if owner[:len(prefix)] != prefix {
		owner = prefix + owner
	}
	createdAt := referral.GetFinalizedTimestamp()
	month := createdAt.Month().String()[:3]
	channel := referral.Channel.Normalize()
	return []Transaction{{
		ID:              referral.GenerateID(),
		CreatedAt:       createdAt,
		Description:     fmt.Sprintf("referrals through %s", month),
		TransactionType: "referral",
		DocumentID:      referral.GetTransactionID(),
		FromAccount:     SettlementAddress,
		FromAccountType: "uphold",
		ToAccount:       owner,
		ToAccountType:   "owner",
		Amount:          altcurrency.BAT.FromProbi(referral.Probi),
		Channel:         &channel,
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
	if referral.GetFinalizedTimestamp().IsZero() {
		errs = append(errs, errors.New("the finalized timestamp from the referral is zero"))
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}
