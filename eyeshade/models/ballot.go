package models

import "github.com/shopspring/decimal"

// Ballot holds informatin about a ballot
type Ballot struct {
	ID         string          `json:"id" db:"id"`
	Cohort     string          `json:"cohort" db:"cohort"`
	Tally      decimal.Decimal `json:"tally" db:"tally"`
	Excluded   bool            `json:"excluded" db:"excluded"`
	Channel    Channel         `json:"channel" db:"channel"`
	SurveyorID string          `json:"surveyorId" db:"surveyor_id"`
}
