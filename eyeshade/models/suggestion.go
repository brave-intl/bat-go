package models

import (
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	// VoteValue holds the bat value of a single vote
	VoteValue = decimal.NewFromFloat(0.25)
)

// Funding holds information about suggestion funding sources
type Funding struct {
	Type      string          `json:"type"`
	Amount    decimal.Decimal `json:"amount"`
	Cohort    string          `json:"cohort"`
	Promotion string          `json:"promotion"`
}

// Suggestion holds information from votes freezing
type Suggestion struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Channel     Channel         `json:"channel"`
	CreatedAt   time.Time       `json:"createdAt"`
	TotalAmount decimal.Decimal `json:"totalAmount"`
	OrderID     string          `json:"orderId"`
	Funding     []Funding       `json:"funding"`
}

// GetSurveyorID returns a surveyor id for a given date
func (funding *Funding) GetSurveyorID(date string) string {
	return fmt.Sprintf("%s_%s", date, funding.Promotion)
}

// GetBallotIDs gets the ids that the votes will be stored under
func (suggestion *Suggestion) GetBallotIDs(date string) []string {
	ids := []string{}
	for _, funding := range suggestion.Funding {
		ids = append(ids, funding.GenerateID(suggestion.Channel, date))
	}
	return ids
}

// ToBallot converts a funding object to a ballot
func (funding *Funding) ToBallot(
	suggestion Suggestion,
	date string,
) Ballot {
	surveyorID := funding.GetSurveyorID(date)
	return Ballot{
		ID:         funding.GenerateID(suggestion.Channel, date),
		Cohort:     funding.Cohort,
		Tally:      funding.Amount.Div(VoteValue),
		Excluded:   false,
		Channel:    suggestion.Channel.Normalize(),
		SurveyorID: surveyorID,
	}
}

// ToSurveyor creates a surveyor from the funding object
func (funding *Funding) ToSurveyor(
	suggestion Suggestion,
	date string,
) Surveyor {
	// minimum values to be inserted into db
	return Surveyor{
		ID:      funding.GetSurveyorID(date),
		Price:   VoteValue,
		Virtual: true,
	}
}

// GetCohort gets the cohort
func (funding *Funding) GetCohort() string {
	return funding.Cohort
}

// GetExcluded gets the excluded value
func (funding *Funding) GetExcluded() bool {
	return false
}

// GenerateID generates an id from the contribution namespace
func (funding *Funding) GenerateID(channel Channel, date string) string {
	surveyorID := funding.GetSurveyorID(date)
	return uuid.NewV5(
		TransactionNS["votes"],
		channel.String()+funding.GetCohort()+surveyorID,
	).String()
}

// CollectBallots collects ballots and surveyors from suggestion
func (suggestion *Suggestion) CollectBallots(
	date string,
	filters ...map[string]bool,
) ([]Surveyor, []Ballot) {
	surveyors := []Surveyor{}
	ballots := []Ballot{}
	s := *suggestion
	surveyorFrozen := map[string]bool{}
	surveyorSeen := map[string]bool{}
	if len(filters) > 0 {
		surveyorFrozen = filters[0]
		if len(filters) > 1 {
			surveyorSeen = filters[1]
		}
	}
	for _, funding := range suggestion.Funding {
		surveyor := funding.ToSurveyor(s, date)
		if !surveyorFrozen[surveyor.ID] && !surveyorSeen[surveyor.ID] {
			surveyors = append(surveyors, surveyor)
		}
		ballot := funding.ToBallot(s, date)
		if surveyorFrozen[ballot.SurveyorID] {
			continue // if already frozen, do not insert
		}
		ballots = append(ballots, ballot)
	}
	return surveyors, ballots
}

// CollectSurveyors collects surveyors from the suggestion
func (suggestion *Suggestion) CollectSurveyors(
	date string,
	filters ...map[string]bool,
) []Surveyor {
	surveyors := []Surveyor{}
	surveyorSeen := map[string]bool{}
	if len(filters) > 0 {
		surveyorSeen = filters[0]
	}
	for _, funding := range suggestion.Funding {
		surveyor := funding.ToSurveyor(*suggestion, date)
		if surveyorSeen[surveyor.ID] {
			continue // if already seen then do not insert
		}
		surveyors = append(surveyors, surveyor)
	}
	return surveyors
}
