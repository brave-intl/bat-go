package models

import (
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	voteValue = decimal.NewFromFloat(0.25)
)

// Funding holds information about suggestion funding sources
type Funding struct {
	Type      string
	Amount    decimal.Decimal
	Cohort    string
	Promotion string
}

// GetSurveyorID returns a surveyor id for a given date
func (funding *Funding) GetSurveyorID(date string) string {
	return fmt.Sprintf("%s_%s", date, funding.Promotion)
}

// SurveyorParams holds parameters needed to ensure that the surveyor exists
func (funding *Funding) SurveyorParams(date string) []interface{} {
	return []interface{}{
		funding.GetSurveyorID(date),
		voteValue,
	}
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

func (funding *Funding) VoteParams(
	suggestion Suggestion,
	date string,
) []interface{} {
	surveyorID := funding.GetSurveyorID(date)
	return []interface{}{
		suggestion.GenerateID(funding.Cohort, surveyorID), // id
		funding.Cohort,                          // cohort
		funding.Amount.Div(voteValue).String(),  // tally
		false,                                   // exclude
		suggestion.Channel.Normalize().String(), // channel
		surveyorID,                              // surveyor id
	}
}

// GenerateID generates an id from the contribution namespace
func (suggestion *Suggestion) GenerateID(cohort, surveyorID string) string {
	return uuid.NewV5(
		TransactionNS["votes"],
		suggestion.Channel.String()+cohort+surveyorID,
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
