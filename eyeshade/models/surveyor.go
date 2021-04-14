package models

import (
	"time"

	"github.com/shopspring/decimal"
)

// Surveyor holds information about surveyors
type Surveyor struct {
	ID        string          `db:"id"`
	Frozen    bool            `db:"frozen"`
	Virtual   bool            `db:"virtual"`
	Price     decimal.Decimal `db:"price"`
	CreatedAt time.Time       `db:"created_at"`
	UpdatedAt time.Time       `db:"updated_at"`
}
