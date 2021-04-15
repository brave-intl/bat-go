package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/countries"
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
	FinalizedTimestamp time.Time               `json:"finalizedTimestamp"`
	ReferralCode       string                  `json:"referralCode"`
	DownloadID         string                  `json:"downloadId"`
	DownloadTimestamp  time.Time               `json:"downloadTimestamp"`
	CountryGroupID     string                  `json:"countryGroupId"`
	Platform           string                  `json:"platform"`
	Amount             decimal.Decimal         `json:"amount"`
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
	createdAt := referral.FinalizedTimestamp
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
		Amount:          referral.Amount,
		Channel:         &channel,
	}}
}

// ToTxIDs create transaction id from a referral
func (referral *Referral) ToTxIDs() []string {
	return []string{referral.GenerateID()}
}

// Ignore allows us to savely ignore the transaction if necessary
func (referral *Referral) Ignore() bool {
	props := referral.Channel.Normalize().Props()
	return referral.Amount.GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the transaction has the necessary data to be inserted correctly
func (referral *Referral) Valid() error {
	errs := []error{}
	if !referral.AltCurrency.IsValid() {
		errs = append(errs, errors.New("altcurrency is not valid"))
	}
	if !referral.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("amount is not greater than zero"))
	}
	if referral.Channel.String() == "" {
		errs = append(errs, errors.New("channel is not set"))
	}
	if referral.Owner == "" {
		errs = append(errs, errors.New("owner is not set"))
	}
	if referral.FinalizedTimestamp.IsZero() {
		errs = append(errs, errors.New("the finalized timestamp from the referral is zero"))
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}

// ReferralBackfill backfills a single referral document with information
// that we do not want to exist on the kafka message
func ReferralBackfill(
	referral *Referral,
	createdAt time.Time,
	group countries.Group,
	rates map[string]decimal.Decimal,
) *Referral {
	referral.FinalizedTimestamp = createdAt
	referral.Amount = group.Amount.Div(rates[group.Currency])
	referral.AltCurrency = altcurrency.BAT
	return referral
}

// ReferralBackfillMany backfills data about settlements from a group and rates
func ReferralBackfillMany(
	referrals *[]Referral,
	groups *[]countries.Group,
	rates map[string]decimal.Decimal,
) (*[]Referral, error) {
	set := []Referral{}
	groupByID := countries.GroupByID(*groups...)
	for _, referral := range *referrals {
		group := groupByID[referral.CountryGroupID]
		if group.Amount.Equal(decimal.Zero) {
			return nil, fmt.Errorf("the country code %s was not found", referral.CountryGroupID)
		}
		set = append(set, *ReferralBackfill(&referral, referral.FinalizedTimestamp, group, rates))
	}
	return &set, nil
}

// ReferralBackfillFromTransactions backfills data about settlements from pre existing transactions
func ReferralBackfillFromTransactions(
	referrals []Referral,
	filled []Transaction,
) []Referral {
	set := []Referral{}
	filledIDMap := map[string]Transaction{}
	for _, tx := range filled {
		filledIDMap[tx.ID] = tx
	}
	for _, referral := range referrals {
		referral.FinalizedTimestamp = filledIDMap[referral.GenerateID()].CreatedAt
		set = append(set, referral)
	}
	return set
}
