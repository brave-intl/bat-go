package eyeshade

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	// SettlementAddress is the address where settlements originate
	SettlementAddress = uphold.SettlementDestination
	largeBAT          = decimal.NewFromFloat(1e9)
	// ErrConvertableFailedValidation when a transaction object fails its validation
	ErrConvertableFailedValidation = errors.New("convertable failed validation")
	// TransactionNS hold a hash of the namespaces for transactions
	TransactionNS = map[string]uuid.UUID{
		"ad":                      uuid.FromStringOrNil("2ca02950-084f-475f-bac3-42a3c99dec95"),
		"contribution":            uuid.FromStringOrNil("be90c1a8-20a3-4f32-be29-ed3329ca8630"),
		"contribution_settlement": uuid.FromStringOrNil("4208cdfc-26f3-44a2-9f9d-1f6657001706"),
		"manual":                  uuid.FromStringOrNil("734a27cd-0834-49a5-8d4c-77da38cdfb22"),
		"manual_settlement":       uuid.FromStringOrNil("a7cb6b9e-b0b4-4c40-85bf-27a0172d4353"),
		"referral":                uuid.FromStringOrNil("3d3e7966-87c3-44ed-84c3-252458f99536"),
		"referral_settlement":     uuid.FromStringOrNil("7fda9071-4f0d-4fe6-b3ac-b1c484d5601a"),
		"settlement_from_channel": uuid.FromStringOrNil("eb296f6d-ab2a-489f-bc75-a34f1ff70acb"),
		"settlement_fees":         uuid.FromStringOrNil("1d295e60-e511-41f5-8ae0-46b6b5d33333"),
		"user_deposit":            uuid.FromStringOrNil("f7a8b983-2383-48f2-9e4f-717f6fe3225d"),
	}
	transactionColumns = []string{
		"id",
		"created_at",
		"description",
		"transaction_type",
		"document_id",
		"from_account",
		"from_account_type",
		"to_account",
		"to_account_type",
		"amount",
		"settlement_currency",
		"settlement_amount",
		"channel",
	}
)

// ConvertableTransaction allows a struct to be converted into a transaction
type ConvertableTransaction interface {
	ToTxs() *[]Transaction
	Valid() bool
	Ignore() bool
}

// Settlement holds information from settlements queue
type Settlement struct {
	AltCurrency    altcurrency.AltCurrency
	Probi          decimal.Decimal
	Fees           decimal.Decimal
	Amount         decimal.Decimal // amount in settlement currency
	Currency       string
	Owner          string
	Channel        Channel
	ID             string
	Type           string
	SettlementID   string
	DocumentID     string
	ExecutedAt     *time.Time
	Address        string
	WalletProvider *string
}

// CreatedAt computes the created at time of the settlement
func (settlement Settlement) CreatedAt() time.Time {
	createdAt := settlement.ExecutedAt
	if createdAt != nil {
		return *createdAt
	}
	parsed, err := time.Parse(time.RFC3339, settlement.ID)
	if err == nil {
		return parsed
	}
	id := settlement.ID
	hexID := hex.EncodeToString([]byte(id))[:8]
	parsedSec, err := strconv.ParseInt(hexID, 16, 64)
	if err != nil {
		return time.Unix(parsedSec, 0)
	}
	return time.Now()
}

// GenerateID generates an id from the settlement metadata
func (settlement Settlement) GenerateID(category string) string {
	return uuid.NewV5(
		TransactionNS[category],
		settlement.SettlementID+settlement.Channel.Normalize().String(),
	).String()
}

// ToTxs converts a settlement to the appropriate transactions
func (settlement Settlement) ToTxs() *[]Transaction {
	txs := []Transaction{}
	createdAt := settlement.CreatedAt()
	month := createdAt.Month().String()[:3]
	normalizedChannel := settlement.Channel.Normalize()
	settlementType := settlement.Type
	transactionType := settlementType + "_settlement"
	toAccountType := "uphold"
	if settlement.WalletProvider != nil {
		toAccountType = *settlement.WalletProvider
	}
	if settlementType == "contribution" {
		txs = append(txs, Transaction{
			ID:              settlement.GenerateID("settlement_from_channel"),
			CreatedAt:       createdAt.Add(time.Second * -2),
			Description:     fmt.Sprintf("contributions through %s", month),
			TransactionType: "contribution",
			DocumentID:      settlement.ID,
			FromAccount:     normalizedChannel.String(),
			FromAccountType: "channel",
			ToAccount:       settlement.Owner,
			ToAccountType:   "owner",
			Amount:          altcurrency.BAT.FromProbi(settlement.Probi.Add(settlement.Fees)),
			Channel:         &normalizedChannel,
		})
		txs = append(txs, Transaction{
			ID:              settlement.GenerateID("settlement_fees"),
			CreatedAt:       createdAt.Add(time.Second * -1),
			Description:     "settlement fees",
			TransactionType: "fees",
			DocumentID:      settlement.ID,
			FromAccount:     settlement.Owner,
			FromAccountType: "owner",
			ToAccount:       "fees-account",
			ToAccountType:   "internal",
			Amount:          altcurrency.BAT.FromProbi(settlement.Fees),
			Channel:         &normalizedChannel,
		})
	} else if settlementType == "manual" {
		txs = append(txs, Transaction{
			ID:              settlement.GenerateID(settlementType),
			CreatedAt:       createdAt, // do we want to change this?
			Description:     "handshake agreement with business development",
			TransactionType: settlementType,
			DocumentID:      settlement.DocumentID,
			FromAccount:     SettlementAddress,
			FromAccountType: toAccountType,
			ToAccount:       settlement.Owner,
			ToAccountType:   "owner",
			Amount:          altcurrency.BAT.FromProbi(settlement.Probi),
		})
	}
	txs = append(txs, Transaction{
		ID:                 settlement.GenerateID(transactionType),
		CreatedAt:          createdAt,
		Description:        fmt.Sprintf("payout for %s", settlement.Type),
		TransactionType:    transactionType,
		FromAccount:        settlement.Owner,
		FromAccountType:    "owner",
		ToAccount:          settlement.Address,
		ToAccountType:      toAccountType, // needs to be updated
		Amount:             altcurrency.BAT.ToProbi(settlement.Probi),
		SettlementCurrency: &settlement.Currency,
		SettlementAmount:   &settlement.Amount,
		Channel:            &normalizedChannel,
	})
	return &txs
}

// Ignore allows us to savely ignore a message if it is malformed
func (settlement Settlement) Ignore() bool {
	props := settlement.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(settlement.Probi).GreaterThan(largeBAT) ||
		altcurrency.BAT.FromProbi(settlement.Fees).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the message it has everything it needs to be inserted correctly
func (settlement Settlement) Valid() bool {
	// non zero and no decimals allowed
	return !settlement.Probi.GreaterThan(decimal.Zero) &&
		settlement.Probi.Equal(settlement.Probi.Truncate(0)) && // no decimals
		settlement.Fees.GreaterThanOrEqual(decimal.Zero) &&
		settlement.Owner != "" &&
		settlement.Channel != "" &&
		settlement.Amount.GreaterThan(decimal.Zero) &&
		settlement.Currency != "" &&
		settlement.Type != "" &&
		settlement.Address != "" &&
		settlement.DocumentID != "" &&
		settlement.SettlementID != ""
}

// Referral holds information from referral queue
type Referral struct {
	AltCurrency   altcurrency.AltCurrency
	Probi         decimal.Decimal
	Channel       Channel
	TransactionID string
	Owner         string
	FirstID       time.Time
}

// GenerateID creates a uuidv5 generated identifier from the referral data
func (referral Referral) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["referral"],
		referral.TransactionID,
	).String()
}

// ToTxs converts a referral to the appropriate transactions
func (referral Referral) ToTxs() *[]Transaction {
	month := referral.FirstID.Month().String()[:3]
	normalizedChannel := referral.Channel.Normalize()
	return &[]Transaction{{
		ID:              referral.GenerateID(),
		CreatedAt:       referral.FirstID,
		Description:     fmt.Sprintf("referrals through %s", month),
		TransactionType: "referral",
		DocumentID:      referral.TransactionID,
		FromAccount:     SettlementAddress,
		FromAccountType: "uphold",
		ToAccount:       referral.Owner,
		ToAccountType:   "owner",
		Amount:          altcurrency.BAT.FromProbi(referral.Probi),
		Channel:         &normalizedChannel,
	}}
}

// Ignore allows us to savely ignore the transaction if necessary
func (referral Referral) Ignore() bool {
	props := referral.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(referral.Probi).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the transaction has the necessary data to be inserted correctly
func (referral Referral) Valid() bool {
	return referral.AltCurrency.IsValid() &&
		referral.Probi.GreaterThan(decimal.Zero) &&
		referral.Probi.Equal(referral.Probi.Truncate(0)) && // no decimals allowed
		referral.Channel != "" &&
		referral.Owner != "" &&
		!referral.FirstID.IsZero()
}

// Votes holds information from votes freezing
type Votes struct {
	Amount            decimal.Decimal
	Fees              decimal.Decimal
	Channel           Channel
	SurveyorID        string
	SurveyorCreatedAt time.Time
}

// GenerateID generates an id from the contribution namespace
func (votes Votes) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["contribution"],
		votes.SurveyorID+votes.Channel.String(),
	).String()
}

// ToTxs converts votes to a list of transactions
func (votes Votes) ToTxs() *[]Transaction {
	return &[]Transaction{{
		ID:              votes.GenerateID(),
		CreatedAt:       votes.SurveyorCreatedAt,
		Description:     fmt.Sprintf("votes from %s", votes.SurveyorID),
		TransactionType: "contribution",
		DocumentID:      votes.SurveyorID,
		FromAccountType: "uphold",
		ToAccount:       votes.Channel.String(),
		ToAccountType:   "channel",
		Amount:          votes.Amount.Add(votes.Fees),
		Channel:         &votes.Channel,
	}}
}

// Ignore allows us to ignore a vote if it is malformed
func (votes Votes) Ignore() bool {
	props := votes.Channel.Normalize().Props()
	return votes.Amount.GreaterThan(largeBAT) ||
		votes.Fees.GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks that we have all of the information needed to insert the transaction
func (votes Votes) Valid() bool {
	return votes.Amount.GreaterThan(decimal.Zero) &&
		votes.SurveyorID != "" &&
		votes.Channel != "" &&
		votes.Fees.GreaterThanOrEqual(decimal.Zero)
}

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
func (userDeposit UserDeposit) GenerateID() string {
	return uuid.NewV5(
		TransactionNS["user_deposit"],
		fmt.Sprintf("%s-%s", userDeposit.Chain, userDeposit.ID),
	).String()
}

// ToTxs converts a user deposit to the appropriate transaction list
func (userDeposit UserDeposit) ToTxs() *[]Transaction {
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
func (userDeposit UserDeposit) Ignore() bool {
	return userDeposit.Amount.GreaterThan(largeBAT)
}

// Valid checks that the transaction has everything it needs before being inserted into the db
func (userDeposit UserDeposit) Valid() bool {
	return userDeposit.CardID != "" &&
		!userDeposit.CreatedAt.IsZero() &&
		userDeposit.Amount.GreaterThan(decimal.Zero) &&
		userDeposit.ID != "" &&
		userDeposit.Chain != "" &&
		userDeposit.Address != ""
}

// AccountEarnings holds results from querying account earnings
type AccountEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
	Earnings  decimal.Decimal `json:"earnings" db:"earnings"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountSettlementEarnings holds results from querying account earnings
type AccountSettlementEarnings struct {
	Channel   Channel         `json:"channel" db:"channel"`
	Paid      decimal.Decimal `json:"paid" db:"paid"`
	AccountID string          `json:"account_id" db:"account_id"`
}

// AccountEarningsOptions receives all options pertaining to account earnings calculations
type AccountEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
}

// AccountSettlementEarningsOptions receives all options pertaining to account settlement earnings calculations
type AccountSettlementEarningsOptions struct {
	Type      string
	Ascending bool
	Limit     int
	StartDate *time.Time
	UntilDate *time.Time
}

// Balance holds information about an account id's balance
type Balance struct {
	AccountID string          `json:"account_id" db:"account_id"`
	Type      string          `json:"account_type" db:"account_type"`
	Balance   decimal.Decimal `json:"balance" db:"balance"`
}

// PendingTransaction holds information about an account id's pending transactions
type PendingTransaction struct {
	Channel Channel         `db:"channel"`
	Balance decimal.Decimal `db:"balance"`
}

// Transaction holds info about a single transaction from the database
type Transaction struct {
	ID                 string           `db:"id"`
	CreatedAt          time.Time        `db:"created_at"`
	Description        string           `db:"description"`
	TransactionType    string           `db:"transaction_type"`
	DocumentID         string           `db:"document_id"`
	FromAccount        string           `db:"from_account"`
	FromAccountType    string           `db:"from_account_type"`
	ToAccount          string           `db:"to_account"`
	ToAccountType      string           `db:"to_account_type"`
	Amount             decimal.Decimal  `db:"amount"`
	SettlementCurrency *string          `db:"settlement_currency"`
	SettlementAmount   *decimal.Decimal `db:"settlement_amount"`
	Channel            *Channel         `db:"channel"`
}

// BackfillForCreators converts a transaction from the database to a backfill transaction
func (tx Transaction) BackfillForCreators(account string) CreatorsTransaction {
	amount := tx.Amount
	if tx.FromAccount == account {
		amount = amount.Neg()
	}
	var settlementDestinationType *string
	var settlementDestination *string
	if settlementTypes[tx.TransactionType] {
		if tx.ToAccountType != "" {
			settlementDestinationType = &tx.ToAccountType
		}
		if tx.ToAccount != "" {
			settlementDestination = &tx.ToAccount
		}
	}
	inputAmount := inputs.NewDecimal(&amount)
	inputSettlementAmount := inputs.NewDecimal(tx.SettlementAmount)
	return CreatorsTransaction{
		Amount:                    inputAmount,
		Channel:                   tx.Channel,
		CreatedAt:                 tx.CreatedAt,
		Description:               tx.Description,
		SettlementCurrency:        tx.SettlementCurrency,
		SettlementAmount:          inputSettlementAmount,
		TransactionType:           tx.TransactionType,
		SettlementDestinationType: settlementDestinationType,
		SettlementDestination:     settlementDestination,
	}
}

// CreatorsTransaction holds a backfilled version of the transaction
type CreatorsTransaction struct {
	CreatedAt                 time.Time      `json:"created_at"`
	Description               string         `json:"description"`
	Channel                   *Channel       `json:"channel"`
	Amount                    inputs.Decimal `json:"amount"`
	TransactionType           string         `json:"transaction_type"`
	SettlementCurrency        *string        `json:"settlement_currency,omitempty"`
	SettlementAmount          inputs.Decimal `json:"settlement_amount,omitempty"`
	SettlementDestinationType *string        `json:"settlement_destination_type,omitempty"`
	SettlementDestination     *string        `json:"settlement_destination,omitempty"`
}

// SettlementStat holds settlement stats
type SettlementStat struct {
	Amount inputs.Decimal `json:"amount" db:"amount"`
}

// SettlementStatOptions holds options for scaling up and down settlement stats
type SettlementStatOptions struct {
	Type     string
	Start    time.Time
	Until    time.Time
	Currency *string
}
