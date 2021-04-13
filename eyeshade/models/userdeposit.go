package models

import (
	"errors"
	"fmt"
	"time"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
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
func (userDeposit *UserDeposit) ToTxs() []Transaction {
	return []Transaction{{
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
func (userDeposit *UserDeposit) Valid() error {
	errs := []error{}
	if userDeposit.CardID == "" {
		errs = append(errs, errors.New("card id is not set"))
	}
	if userDeposit.CreatedAt.IsZero() {
		errs = append(errs, errors.New("created at is not set"))
	}
	if !userDeposit.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("amount is not greater than zero"))
	}
	if userDeposit.ID == "" {
		errs = append(errs, errors.New("id is not set"))
	}
	if userDeposit.Chain == "" {
		errs = append(errs, errors.New("chain is not set"))
	}
	if userDeposit.Address == "" {
		errs = append(errs, errors.New("address is not set"))
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}
