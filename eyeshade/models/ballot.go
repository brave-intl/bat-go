package models

import (
	"errors"
	"fmt"
	"time"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	// ErrBallotAmount means the amount in the ballot is too low
	ErrBallotAmount = errors.New("ballot amount is too low")
	// ErrBallotSurveyorCreatedAt ballot is missing the surveyor created at value
	ErrBallotSurveyorCreatedAt = errors.New("ballot missing surveyor created at timestamp")
	testCohorts                = map[string]bool{
		"test": true,
	}
)

// Ballot holds informatin about a ballot
type Ballot struct {
	ID                string           `json:"id" db:"id"`
	Cohort            string           `json:"cohort" db:"cohort"`
	Tally             int              `json:"tally" db:"tally"`
	Excluded          bool             `json:"excluded" db:"excluded"`
	Channel           Channel          `json:"channel" db:"channel"`
	SurveyorID        string           `json:"surveyorId" db:"surveyor_id"`
	Amount            *decimal.Decimal `json:"amount" db:"amount,omitempty"`
	Fees              *decimal.Decimal `json:"fees" db:"fees,omitempty"`
	SurveyorCreatedAt *time.Time       `json:"-"`
}

// GenerateID generates an id from the ballot
func (ballot *Ballot) GenerateID() string {
	return uuid.NewV5(
		TransactionNS[TransactionTypes.Contribution],
		ballot.SurveyorID+ballot.Channel.Normalize().String(),
	).String()
}

// ToTxIDs returns a list of the transaction ids for a given ballot
func (ballot *Ballot) ToTxIDs() []string {
	return []string{ballot.GenerateID()}
}

// Ignore ignores the convertable transaction
func (ballot *Ballot) Ignore() bool {
	return testCohorts[ballot.Cohort]
}

// Valid checks if the ballot is valid
func (ballot *Ballot) Valid() error {
	errs := []error{}
	if !ballot.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, ErrBallotAmount)
	}
	if ballot.SurveyorCreatedAt == nil {
		errs = append(errs, ErrBallotSurveyorCreatedAt)
	}
	if len(errs) > 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}

// ToTxs creates transactions from the ballot
func (ballot *Ballot) ToTxs() []Transaction {
	normalizedChannel := ballot.Channel.Normalize()
	return []Transaction{{
		ID:              ballot.GenerateID(),
		CreatedAt:       *ballot.SurveyorCreatedAt,
		Description:     fmt.Sprintf("votes from %s", ballot.SurveyorID),
		TransactionType: TransactionTypes.Contribution,
		DocumentID:      ballot.SurveyorID,
		FromAccount:     KnownAccounts.SettlementAddress,
		FromAccountType: AccountTypes.Uphold,
		ToAccount:       normalizedChannel.String(),
		ToAccountType:   AccountTypes.Channel,
		Amount:          ballot.Amount.Add(*ballot.Fees),
		Channel:         &normalizedChannel,
	}}
}

// BallotsToConvertableTransactions converts ballots to convertable transactions
func BallotsToConvertableTransactions(
	ballots ...Ballot,
) []ConvertableTransaction {
	convertables := []ConvertableTransaction{}
	for i := range ballots {
		convertables = append(convertables, &ballots[i])
	}
	return convertables
}
