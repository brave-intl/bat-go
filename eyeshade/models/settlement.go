package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/inputs"
	stringutils "github.com/brave-intl/bat-go/utils/string"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

var (
	suffix = "_settlement"
	// SettlementKeys holds settlement keys to make using them foolproof
	SettlementKeys = keys{
		SettlementFromChannel: "settlement_from_channel",
		SettlementFees:        "settlement_fees",
	}
	// TransactionTypes holds different settlement types that eyeshade can handle
	TransactionTypes = transactionTypes{
		Contribution: "contribution",
		Referral:     "referral",
		Manual:       "manual",
		Fees:         "fees",
		UserDeposit:  "user_deposit",
		Scaleup:      "scaleup",
		Ad:           "ad",
	}
	// TransactionTypesList a list version of the transaction types
	TransactionTypesList = stringutils.CollectValues(TransactionTypes)
	// AccountTypes holds possible account types
	AccountTypes = accountTypes{
		Channel:   "channel",
		Owner:     "owner",
		Uphold:    "uphold",
		Internal:  "internal",
		PaymentID: "payment_id",
		Bitcoin:   "bitcoin",
		Litecoin:  "litecoin",
		Ethereum:  "ethereum",
	}
	// Providers holds possible known providers
	Providers = providers{
		Uphold:   "uphold",
		Gemini:   "gemini",
		BitFlyer: "bitflyer",
	}
	// KnownAccounts holds accounts that are fixed values
	KnownAccounts = knownAccounts{
		FeesAccount:       "fees-account",
		SettlementAddress: SettlementAddress,
	}
)

type knownAccounts struct {
	FeesAccount       string
	SettlementAddress string
}

type providers struct {
	Uphold   string
	Gemini   string
	BitFlyer string
}

type accountTypes struct {
	Owner     string
	Channel   string
	Internal  string
	Uphold    string
	PaymentID string
	Bitcoin   string
	Litecoin  string
	Ethereum  string
}

type transactionTypes struct {
	Manual       string
	Contribution string
	Referral     string
	Fees         string
	UserDeposit  string
	Scaleup      string
	Ad           string
}

type keys struct {
	SettlementFromChannel string
	SettlementFees        string
}

// AddSuffix adds the settlement suffix to a string
func (k *keys) AddSuffix(key string) string {
	return key + suffix
}

// IsSettlementTypeSuffixPresent checks if the value has the _settlement suffix
func IsSettlementTypeSuffixPresent(t string) bool {
	lenT := len(t)
	lenS := len(suffix)
	return lenT >= lenS && suffix == t[lenT-lenS:]
}

// Settlement holds information from settlements queue
type Settlement struct {
	AltCurrency    altcurrency.AltCurrency `json:"altcurrency"`
	Probi          decimal.Decimal         `json:"probi"`
	Fees           decimal.Decimal         `json:"fees"`
	Fee            decimal.Decimal         `json:"fee"`
	Commission     decimal.Decimal         `json:"commission"`
	Amount         decimal.Decimal         `json:"amount"` // amount in settlement currency
	Currency       string                  `json:"currency"`
	Owner          string                  `json:"owner"`
	Channel        Channel                 `json:"publisher"`
	Hash           string                  `json:"hash"`
	Type           string                  `json:"type"`
	SettlementID   string                  `json:"settlementId"`
	DocumentID     string                  `json:"documentId"`
	Address        string                  `json:"address"`
	ExecutedAt     string                  `json:"executedAt"`
	WalletProvider string                  `json:"walletProvider"`
}

// GenerateID generates an id from the settlement metadata
func (settlement *Settlement) GenerateID(category string) string {
	return uuid.NewV5(
		TransactionNS[category],
		settlement.SettlementID+settlement.Channel.Normalize().String(),
	).String()
}

// ToTxIDs create transaction ids from a settlemen
func (settlement *Settlement) ToTxIDs() []string {
	keys := []string{
		SettlementKeys.AddSuffix(settlement.Type),
	}
	if settlement.Type == TransactionTypes.Contribution {
		keys = append(
			keys,
			SettlementKeys.SettlementFromChannel,
			SettlementKeys.SettlementFees,
		)
	} else if settlement.Type == TransactionTypes.Manual {
		// be careful when using manual transactions this will fail
		// TransactionNS does not have the appropriate key
		keys = append(keys, settlement.Type) // straight pass through
	}
	ids := []string{}
	for _, key := range keys {
		ids = append(ids, settlement.GenerateID(key))
	}
	return ids
}

// ToContributionTransactions creates a set of contribution transactions for settlements
func (settlement *Settlement) ToContributionTransactions() []Transaction {
	createdAt := settlement.GetCreatedAt()
	month := createdAt.Month().String()[:3]
	normalizedChannel := settlement.Channel.Normalize()
	return []Transaction{{
		ID:              settlement.GenerateID(SettlementKeys.SettlementFromChannel),
		CreatedAt:       createdAt.Add(time.Second * -2),
		Description:     fmt.Sprintf("contributions through %s", month),
		TransactionType: TransactionTypes.Contribution,
		DocumentID:      settlement.Hash,
		FromAccount:     normalizedChannel.String(),
		FromAccountType: AccountTypes.Channel,
		ToAccount:       settlement.Owner,
		ToAccountType:   AccountTypes.Owner,
		Amount:          altcurrency.BAT.FromProbi(settlement.Probi.Add(settlement.Fees)),
		Channel:         &normalizedChannel,
	}, {
		ID:              settlement.GenerateID(SettlementKeys.SettlementFees),
		CreatedAt:       createdAt.Add(time.Second * -1),
		Description:     "settlement fees",
		TransactionType: TransactionTypes.Fees,
		DocumentID:      settlement.Hash,
		FromAccount:     settlement.Owner,
		FromAccountType: AccountTypes.Owner,
		ToAccount:       KnownAccounts.FeesAccount,
		ToAccountType:   AccountTypes.Internal,
		Amount:          altcurrency.BAT.FromProbi(settlement.Fees),
		Channel:         &normalizedChannel,
	}}
}

// GetCreatedAt gets the time the message was created at
func (settlement *Settlement) GetCreatedAt() time.Time {
	createdAt, _ := time.Parse(time.RFC3339, settlement.ExecutedAt)
	return createdAt
}

// GetToAccountType gets the account type
func (settlement *Settlement) GetToAccountType() string {
	toAccountType := AccountTypes.Uphold
	if settlement.WalletProvider != "" {
		toAccountType = settlement.WalletProvider
	}
	return toAccountType
}

// ToManualTransaction creates a manual transaction
func (settlement *Settlement) ToManualTransaction() Transaction {
	return Transaction{
		ID:              settlement.GenerateID(settlement.Type),
		CreatedAt:       settlement.GetCreatedAt(), // do we want to change this?
		Description:     "handshake agreement with business development",
		TransactionType: settlement.Type,
		DocumentID:      settlement.DocumentID,
		FromAccount:     KnownAccounts.SettlementAddress,
		FromAccountType: settlement.GetToAccountType(),
		ToAccount:       settlement.Owner,
		ToAccountType:   AccountTypes.Owner,
		Amount:          altcurrency.BAT.FromProbi(settlement.Probi),
	}
}

// ToSettlementTransaction creates a settlement transaction
func (settlement *Settlement) ToSettlementTransaction() Transaction {
	normalizedChannel := settlement.Channel.Normalize()
	settlementType := settlement.Type
	transactionType := SettlementKeys.AddSuffix(settlementType)
	documentID := settlement.DocumentID
	if settlement.Type != TransactionTypes.Manual {
		documentID = settlement.Hash
	}
	return Transaction{
		ID:                 settlement.GenerateID(transactionType),
		CreatedAt:          settlement.GetCreatedAt(),
		Description:        fmt.Sprintf("payout for %s", settlement.Type),
		TransactionType:    transactionType,
		FromAccount:        settlement.Owner,
		DocumentID:         documentID,
		FromAccountType:    AccountTypes.Owner,
		ToAccount:          settlement.Address,
		ToAccountType:      settlement.GetToAccountType(), // needs to be updated
		Amount:             altcurrency.BAT.FromProbi(settlement.Probi),
		SettlementCurrency: &settlement.Currency,
		SettlementAmount:   &settlement.Amount,
		Channel:            &normalizedChannel,
	}
}

// ToTxs converts a settlement to the appropriate transactions
func (settlement *Settlement) ToTxs() []Transaction {
	txs := []Transaction{}
	if settlement.Type == TransactionTypes.Contribution {
		txs = append(txs, settlement.ToContributionTransactions()...)
	} else if settlement.Type == TransactionTypes.Manual {
		txs = append(txs, settlement.ToManualTransaction())
	}
	return append(txs, settlement.ToSettlementTransaction())
}

// Ignore allows us to savely ignore a message if it is malformed
func (settlement *Settlement) Ignore() bool {
	props := settlement.Channel.Normalize().Props()
	return altcurrency.BAT.FromProbi(settlement.Probi).GreaterThan(largeBAT) ||
		altcurrency.BAT.FromProbi(settlement.Fees).GreaterThan(largeBAT) ||
		(props.ProviderName == "youtube" && props.ProviderSuffix == "user")
}

// Valid checks whether the message it has everything it needs to be inserted correctly
func (settlement *Settlement) Valid() error {
	// non zero and no decimals allowed
	errs := []error{}
	if !settlement.Probi.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("probi is not greater than zero"))
	}
	if !settlement.Probi.Equal(settlement.Probi.Truncate(0)) {
		errs = append(errs, errors.New("probi is not an int"))
	}
	if !settlement.Fees.GreaterThanOrEqual(decimal.Zero) {
		errs = append(errs, errors.New("fees are negative"))
	}
	if settlement.Owner == "" {
		errs = append(errs, errors.New("owner not set"))
	}
	if settlement.Channel.String() == "" {
		errs = append(errs, errors.New("channel not set"))
	}
	if !settlement.Amount.GreaterThan(decimal.Zero) {
		errs = append(errs, errors.New("amount is not greater than zero"))
	}
	if settlement.Currency == "" {
		errs = append(errs, errors.New("currency is not set"))
	}
	if settlement.Type == "" {
		errs = append(errs, errors.New("transaction type is not set"))
	}
	if settlement.Address == "" {
		errs = append(errs, errors.New("address is not set"))
	}
	if settlement.DocumentID == "" {
		errs = append(errs, errors.New("document id is not set"))
	}
	if settlement.SettlementID == "" {
		errs = append(errs, errors.New("settlement id is not set"))
	}
	if len(errs) != 0 {
		return &errorutils.MultiError{
			Errs: errs,
		}
	}
	return nil
}

// SettlementStat holds settlement stats
type SettlementStat struct {
	Amount inputs.Decimal `json:"amount" db:"amount"`
}

// SettlementStatOptions holds options for scaling up and down settlement stats
type SettlementStatOptions struct {
	Type     string
	Start    inputs.Time
	Until    inputs.Time
	Currency *string
}

// NewSettlementStatOptions creates a bundle of settlement stat options
func NewSettlementStatOptions(
	t, currencyInput, layout, start, until string,
) (*SettlementStatOptions, error) {
	startTime, untilTime, err := parseTimes(layout, start, until)
	if err != nil {
		return nil, err
	}
	var currency *string
	if currencyInput != "" {
		currency = &currencyInput
	}
	return &SettlementStatOptions{
		Type:     t,
		Start:    *startTime,
		Until:    *untilTime,
		Currency: currency,
	}, nil
}

// GrantStat holds Grant stats
type GrantStat struct {
	Amount inputs.Decimal `json:"amount" db:"amount"`
	Count  inputs.Decimal `json:"count" db:"count"`
}

// GrantStatOptions holds options for scaling up and down grant stats
type GrantStatOptions struct {
	Type  string
	Start inputs.Time
	Until inputs.Time
}

// NewGrantStatOptions creates a new grant stat options object
func NewGrantStatOptions(
	t, layout, start, until string,
) (*GrantStatOptions, error) {
	startTime, untilTime, err := parseTimes(layout, start, until)
	if err != nil {
		return nil, err
	}
	if !ValidGrantStatTypes[t] {
		return nil, fmt.Errorf("unable to check grant stats for type %s", t)
	}
	return &GrantStatOptions{
		Type:  t,
		Start: *startTime,
		Until: *untilTime,
	}, nil
}

func parseTimes(
	layout, start, until string,
) (*inputs.Time, *inputs.Time, error) {
	var (
		startTime = inputs.NewTime(layout)
		untilTime = inputs.NewTime(layout)
		ctx       = context.Background()
	)
	if err := startTime.Decode(ctx, []byte(start)); err != nil {
		return nil, nil, err
	}
	if err := untilTime.Decode(ctx, []byte(until)); err != nil {
		untilSubMonth := startTime.Time().AddDate(0, -1, 0)
		untilTime.SetTime(untilSubMonth)
	}
	return startTime, untilTime, nil
}

// SettlementBackfill backfills data on the settlement object
func SettlementBackfill(settlement Settlement, t ...time.Time) Settlement {
	if settlement.ExecutedAt == "" {
		if len(t) > 0 {
			// use the time that the message was placed on the queue if none inside of msg
			settlement.ExecutedAt = t[0].Format(time.RFC3339)
		}
	}
	if settlement.WalletProvider == "" {
		settlement.WalletProvider = Providers.Uphold
	}
	return settlement
}

// SettlementBackfillMany backfills with now as the timestamp
func SettlementBackfillMany(settlements []Settlement) []Settlement {
	s := []Settlement{}
	now := time.Now()
	for _, settlement := range settlements {
		s = append(s, SettlementBackfill(settlement, now))
	}
	return s
}

// SettlementBackfillFromTransactions backfills data about settlements from pre existing transactions
func SettlementBackfillFromTransactions(
	settlements []Settlement,
	filled []Transaction,
) []Settlement {
	set := []Settlement{}
	filledIDMap := map[string]Transaction{}
	for _, tx := range filled {
		filledIDMap[tx.ID] = tx
	}
	for _, settlement := range settlements {
		transactionType := SettlementKeys.AddSuffix(settlement.Type)
		filledTx := filledIDMap[settlement.GenerateID(transactionType)]
		set = append(set, SettlementBackfill(settlement, filledTx.CreatedAt))
	}
	return set
}
