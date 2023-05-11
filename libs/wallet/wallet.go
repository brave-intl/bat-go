// Package wallet defines common datastructures and an interface for cryptocurrency wallets
package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// Info contains information about a wallet like associated identifiers, the denomination,
// the last known balance and provider
type Info struct {
	ID                         string                   `json:"paymentId" valid:"uuidv4,optional" db:"id"`
	Provider                   string                   `json:"provider" valid:"in(uphold,brave)" db:"provider"`
	ProviderID                 string                   `json:"providerId" valid:"uuidv4" db:"provider_id"`
	AltCurrency                *altcurrency.AltCurrency `json:"altcurrency" valid:"-"`
	PublicKey                  string                   `json:"publicKey,omitempty" valid:"hexadecimal,optional" db:"public_key"`
	LastBalance                *Balance                 `json:"balances,omitempty" valid:"-"`
	ProviderLinkingID          *uuid.UUID               `json:"providerLinkingId" valid:"-" db:"provider_linking_id"`
	AnonymousAddress           *uuid.UUID               `json:"anonymousAddress" valid:"-" db:"anonymous_address"`
	UserDepositAccountProvider *string                  `json:"userDepositAccountProvider" valid:"in(uphold)" db:"user_deposit_account_provider"`
	UserDepositDestination     string                   `json:"userDepositCardId" db:"user_deposit_destination"`
}

// TransactionInfo contains information about a transaction like the denomination, amount in probi,
// destination address, status and identifier
type TransactionInfo struct {
	Probi              decimal.Decimal          `json:"probi"`
	AltCurrency        *altcurrency.AltCurrency `json:"altcurrency"`
	Destination        string                   `json:"address"`
	TransferFee        decimal.Decimal          `json:"fee"`
	ExchangeFee        decimal.Decimal          `json:"-"`
	Status             string                   `json:"status"`
	ID                 string                   `json:"id"`
	DestCurrency       string                   `json:"-"`
	DestAmount         decimal.Decimal          `json:"-"`
	ValidUntil         time.Time                `json:"-"`
	Source             string                   `json:"-"`
	Time               time.Time                `json:"-"`
	Note               string                   `json:"-"`
	UserID             string                   `json:"-"`
	KYC                bool                     `json:"-"`
	CitizenshipCountry string                   `json:"-"`
	IdentityCountry    string                   `json:"-"`
	ResidenceCountry   string                   `json:"-"`
}

// String returns the transaction info as an easily readable string
func (t TransactionInfo) String() string {
	return fmt.Sprintf("%s: %s %s sent from %s to %s, charged transfer fee %s and exchange fee %s, destination recieved %s %s", t.Time,
		t.AltCurrency.FromProbi(t.Probi), t.AltCurrency, t.Source, t.Destination, t.TransferFee, t.ExchangeFee, t.DestAmount, t.DestCurrency)
}

// ByTime implements sort.Interface for []TransactionInfo based on the Time field.
type ByTime []TransactionInfo

func (a ByTime) Len() int           { return len(a) }
func (a ByTime) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByTime) Less(i, j int) bool { return a[i].Time.Before(a[j].Time) }

// Balance holds balance information for a wallet
type Balance struct {
	TotalProbi       decimal.Decimal
	SpendableProbi   decimal.Decimal
	ConfirmedProbi   decimal.Decimal
	UnconfirmedProbi decimal.Decimal
}

// Wallet is an interface for a cryptocurrency wallet
type Wallet interface {
	GetWalletInfo() Info
	// Transfer moves funds out of the associated wallet and to the specific destination
	Transfer(ctx context.Context, altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*TransactionInfo, error)
	// VerifyTransaction verifies that the base64 encoded transaction is valid
	// NOTE VerifyTransaction must guard against transactions that seek to exploit parser differences
	// such as including additional fields that are not understood by local implementation but may
	// be understood by the upstream wallet provider.
	VerifyTransaction(ctx context.Context, transactionB64 string) (*TransactionInfo, error)
	// SubmitTransaction submits the base64 encoded transaction for verification but does not move funds
	SubmitTransaction(ctx context.Context, transactionB64 string, confirm bool) (*TransactionInfo, error)
	// ConfirmTransaction confirms a previously submitted transaction, moving funds
	ConfirmTransaction(ctx context.Context, id string) (*TransactionInfo, error)
	// GetBalance returns the last known balance, if refresh is true then the current balance is fetched
	GetBalance(ctx context.Context, refresh bool) (*Balance, error)
	// ListTransactions for this wallet, limit number of transactions returned
	ListTransactions(ctx context.Context, limit int, startDate time.Time) ([]TransactionInfo, error)
}
