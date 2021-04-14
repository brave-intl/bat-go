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

// ToBallot converts a funding object to a ballot
func (funding *Funding) ToBallot(
	suggestion Suggestion,
	date string,
) Ballot {
	surveyorID := funding.GetSurveyorID(date)
	return Ballot{
		ID:         funding.GenerateID(suggestion.Channel, date),
		Cohort:     funding.Cohort,
		Tally:      funding.Amount.Div(voteValue),
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
		Price:   voteValue,
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
	surveyorFrozen, surveyorSeen map[string]bool,
	date string,
) ([]Surveyor, []Ballot) {
	surveyors := []Surveyor{}
	ballots := []Ballot{}
	s := *suggestion
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
	surveyorSeen map[string]bool,
	date string,
) []Surveyor {
	surveyors := []Surveyor{}
	for _, funding := range suggestion.Funding {
		surveyor := funding.ToSurveyor(*suggestion, date)
		if surveyorSeen[surveyor.ID] {
			continue // if already seen then do not insert
		}
		surveyors = append(surveyors, surveyor)
	}
	return surveyors
}
