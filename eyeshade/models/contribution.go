package models

import (
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Contribution holds information from user contribution
type Contribution struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Channel       Channel         `json:"channel"`
	CreatedAt     time.Time       `json:"createdAt"`
	BaseVoteValue decimal.Decimal `json:"baseVoteValue"`
	VoteTally     decimal.Decimal `json:"voteTally"`
	FundingSource string          `json:"fundingSource"`
}

// GetSurveyorID gets the surveyor id
func (contribution *Contribution) GetSurveyorID(date string) string {
	d := contribution.CreatedAt.Format(time.RFC3339)
	if date != "" {
		d = date
	}
	return fmt.Sprintf("%s_%s", d, contribution.FundingSource)
}

// ToSurveyor creates a surveyor to make sure the vote can be seen
func (contribution *Contribution) ToSurveyor(date string) Surveyor {
	return Surveyor{
		ID:      contribution.GetSurveyorID(date),
		Price:   contribution.BaseVoteValue,
		Virtual: true,
	}
}

// GetCohort gets the cohort value
func (contribution *Contribution) GetCohort() string {
	return "control"
}

// GetExcluded gets the excluded value
func (contribution *Contribution) GetExcluded() bool {
	// if you ever want to run tests, you can make edits here and votes will not show
	return false
}

// ToBallot creates a ballot from the contribution
func (contribution *Contribution) ToBallot(date string) Ballot {
	return Ballot{
		ID:         contribution.GenerateID(date),
		Cohort:     contribution.GetCohort(),
		Tally:      contribution.VoteTally,
		Excluded:   contribution.GetExcluded(),
		Channel:    contribution.Channel.Normalize(),
		SurveyorID: contribution.GetSurveyorID(date),
	}
}

// GenerateID generates an id from the contribution namespace
func (contribution *Contribution) GenerateID(date string) string {
	return uuid.NewV5(
		TransactionNS["votes"],
		contribution.Channel.Normalize().String()+contribution.GetCohort()+contribution.GetSurveyorID(date),
	).String()
}

// CollectBallots collects surveyors and ballot types
// filters can be passed to exclude the surveyor or both
func (contribution *Contribution) CollectBallots(
	surveyorFrozen, surveyorSeen map[string]bool,
	date string,
) ([]Surveyor, []Ballot) {
	surveyors := []Surveyor{}
	ballots := []Ballot{}
	surveyor := contribution.ToSurveyor(date)
	if !surveyorFrozen[surveyor.ID] && !surveyorSeen[surveyor.ID] {
		surveyors = []Surveyor{surveyor}
	}
	ballot := contribution.ToBallot(date)
	if !surveyorFrozen[surveyor.ID] {
		ballots = []Ballot{ballot}
	}
	return surveyors, ballots
}

// CollectSurveyors collects the surveyors from the contribution
func (contribution *Contribution) CollectSurveyors(
	surveyorSeen map[string]bool,
	date string,
) []Surveyor {
	surveyor := contribution.ToSurveyor(date)
	if surveyorSeen[surveyor.ID] {
		return []Surveyor{}
	}
	return []Surveyor{surveyor}
}
