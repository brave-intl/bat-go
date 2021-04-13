package models

import (
	"errors"
	"fmt"
	"time"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Votes holds information from votes freezing
type Votes struct {
	Amount            decimal.Decimal
	Fees              decimal.Decimal
	Channel           Channel
	SurveyorID        string
	SurveyorCreatedAt time.Time
}

// GenerateID generates an id from the contribution namespace
func (votes *Votes) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["contribution"],
		votes.SurveyorID+votes.Channel.String(),
	).String()
}

// ToTxs converts votes to a list of transactions
func (votes *Votes) ToTxs() []Transaction {
	return []Transaction{{
		ID:              votes.GenerateID(),
		CreatedAt:       votes.SurveyorCreatedAt,
		Description:     fmt.Sprintf("votes from %s", votes.SurveyorID),
		TransactionType: "contribution",
		DocumentID:      votes.SurveyorID,
		FromAccountType: "uphold",
		ToAccount:       votes.Channel.String(),
		ToAccountType:   "channel",
		Amount:          votes.Amount.Add(votes.Fees),
		Channel:         &votes.Channel,
	}}
}

// Ignore allows us to ignore a vote if it is malformed
func (votes *Votes) Ignore() bool {
	props := votes.Channel.Normalize().Props()
	return votes.Amount.GreaterThan(largeBAT) ||
		votes.Fees.GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks that we have all of the information needed to insert the transaction
func (votes *Votes) Valid() error {
	errs := []error{}
	if !votes.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("vote amount is not greater than zero"))
	}
	if votes.SurveyorID == "" {
		errs = append(errs, errors.New("surveyor id is not set"))
	}
	if votes.Channel.String() == "" {
		errs = append(errs, errors.New("channel is not set"))
	}
	if !votes.Fees.GreaterThanOrEqual(decimal.Zero) {
		errs = append(errs, errors.New("fees are negative"))
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}
