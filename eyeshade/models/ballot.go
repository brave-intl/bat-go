package models

import "github.com/shopspring/decimal"

// Ballot holds informatin about a ballot
type Ballot struct {
	ID         string          `db:"id"`
	Cohort     string          `db:"cohort"`
	Tally      decimal.Decimal `db:"tally"`
	Excluded   bool            `db:"excluded"`
	Channel    Channel         `db:"channel"`
	SurveyorID string          `db:"surveyor_id"`
}
