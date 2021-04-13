package models

import (
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Funding holds information about suggestion funding sources
type Funding struct {
	Type      string
	Amount    decimal.Decimal
	Cohort    string
	Promotion string
}

// Suggestion holds information from votes freezing
type Suggestion struct {
	ID          string
	Type        string
	Channel     Channel
	CreatedAt   time.Time
	TotalAmount decimal.Decimal
	OrderID     string
	Funding     []Funding
}

// GenerateID generates an id from the contribution namespace
func (votes *Suggestion) GenerateID(cohort, surveyorID string) string {
	return uuid.NewV5(
		TransactionNS["votes"],
		votes.Channel.String()+cohort+surveyorID,
	).String()
}

// // ToTxs converts votes to a list of transactions
// func (votes *Suggestion) ToTxs() []Transaction {
// 	return []Transaction{{
// 		ID:              votes.GenerateID(),
// 		CreatedAt:       votes.SurveyorCreatedAt,
// 		Description:     fmt.Sprintf("votes from %s", votes.SurveyorID),
// 		TransactionType: "contribution",
// 		DocumentID:      votes.SurveyorID,
// 		FromAccountType: "uphold",
// 		ToAccount:       votes.Channel.String(),
// 		ToAccountType:   "channel",
// 		Amount:          votes.Amount.Add(votes.Fees),
// 		Channel:         &votes.Channel,
// 	}}
// }

// // Ignore allows us to ignore a vote if it is malformed
// func (votes *Suggestion) Ignore() bool {
// 	props := votes.Channel.Normalize().Props()
// 	return votes.Amount.GreaterThan(largeBAT) ||
// 		votes.Fees.GreaterThan(largeBAT) ||
// 		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
// }

// // Valid checks that we have all of the information needed to insert the transaction
// func (votes *Suggestion) Valid() error {
// 	errs := []error{}
// 	if !votes.Amount.GreaterThan(decimal.Zero) {
// 		errs = append(errs, errors.New("vote amount is not greater than zero"))
// 	}
// 	if votes.SurveyorID == "" {
// 		errs = append(errs, errors.New("surveyor id is not set"))
// 	}
// 	if votes.Channel.String() == "" {
// 		errs = append(errs, errors.New("channel is not set"))
// 	}
// 	if !votes.Fees.GreaterThanOrEqual(decimal.Zero) {
// 		errs = append(errs, errors.New("fees are negative"))
// 	}
// 	if len(errs) > 0 {
// 		return &errorutils.MultiError{
// 			Errs: errs,
// 		}
// 	}
// 	return nil
// }
