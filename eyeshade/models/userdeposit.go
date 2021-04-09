package models

import (
	"fmt"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// UserDeposit holds information from user deposits
type UserDeposit struct {
	ID        string
	Amount    decimal.Decimal
	Chain     string
	CardID    string
	CreatedAt time.Time
	Address   string
}

// GenerateID creates a uuidv5 generated identifier from the user deposit data
func (userDeposit *UserDeposit) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["user_deposit"],
		fmt.Sprintf("%s-%s", userDeposit.Chain, userDeposit.ID),
	).String()
}

// ToTxs converts a user deposit to the appropriate transaction list
func (userDeposit *UserDeposit) ToTxs() *[]Transaction {
	return &[]Transaction{{
		ID:              userDeposit.GenerateID(),
		CreatedAt:       userDeposit.CreatedAt,
		Description:     fmt.Sprintf("deposits from %s chain", userDeposit.Chain),
		TransactionType: "user_deposit",
		DocumentID:      userDeposit.ID,
		FromAccount:     userDeposit.Address,
		FromAccountType: userDeposit.Chain,
		ToAccount:       userDeposit.CardID,
		ToAccountType:   "uphold",
		Amount:          userDeposit.Amount,
	}}
}

// Ignore allows us to savely ignore it
func (userDeposit *UserDeposit) Ignore() bool {
	return userDeposit.Amount.GreaterThan(largeBAT)
}

// Valid checks that the transaction has everything it needs before being inserted into the db
func (userDeposit *UserDeposit) Valid() bool {
	return userDeposit.CardID != "" &&
		!userDeposit.CreatedAt.IsZero() &&
		userDeposit.Amount.GreaterThan(decimal.Zero) &&
		userDeposit.ID != "" &&
		userDeposit.Chain != "" &&
		userDeposit.Address != ""
}
