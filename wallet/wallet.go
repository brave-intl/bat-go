// Package wallet defines common datastructures and an interface for cryptocurrency wallets
package wallet

import (
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

// Info contains information about a wallet like associated identifiers, the denomination,
// the last known balance and provider
type Info struct {
	ID          string                   `json:"paymentId" valid:"uuidv4,optional" db:"id"`
	Provider    string                   `json:"provider" valid:"in(uphold)" db:"provider"`
	ProviderID  string                   `json:"providerId" valid:"uuidv4" db:"provider_id"`
	AltCurrency *altcurrency.AltCurrency `json:"altcurrency" valid:"-"`
	PublicKey   string                   `json:"publicKey,omitempty" valid:"hexadecimal,optional" db:"public_key"`
	LastBalance *Balance                 `json:"balances,omitempty" valid:"-"`
}

// TransactionInfo contains information about a transaction like the denomination, amount in probi,
// destination address, status and identifier
type TransactionInfo struct {
	Probi        decimal.Decimal          `json:"probi"`
	AltCurrency  *altcurrency.AltCurrency `json:"altcurrency"`
	Destination  string                   `json:"address"`
	TransferFee  decimal.Decimal          `json:"fee"`
	ExchangeFee  decimal.Decimal          `json:"-"`
	Status       string                   `json:"status"`
	ID           string                   `json:"id"`
	DestCurrency string                   `json:"-"`
	DestAmount   decimal.Decimal          `json:"-"`
	ValidUntil   time.Time                `json:"-"`
	Source       string                   `json:"-"`
	Time         time.Time                `json:"-"`
	Note         string                   `json:"-"`
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
	Transfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*TransactionInfo, error)
	// VerifyTransaction verifies that the base64 encoded transaction is valid
	// NOTE VerifyTransaction must guard against transactions that seek to exploit parser differences
	// such as including additional fields that are not understood by local implementation but may
	// be understood by the upstream wallet provider.
	VerifyTransaction(transactionB64 string) (*TransactionInfo, error)
	// SubmitTransaction submits the base64 encoded transaction for verification but does not move funds
	SubmitTransaction(transactionB64 string, confirm bool) (*TransactionInfo, error)
	// ConfirmTransaction confirms a previously submitted transaction, moving funds
	ConfirmTransaction(id string) (*TransactionInfo, error)
	// GetBalance returns the last known balance, if refresh is true then the current balance is fetched
	GetBalance(refresh bool) (*Balance, error)
	// ListTransactions for this wallet, limit number of transactions returned
	ListTransactions(limit int) ([]TransactionInfo, error)
}

// IsNotFound is a helper method for determining if an error indicates a missing resource
func IsNotFound(err error) bool {
	type notFound interface {
		NotFoundError() bool
	}
	te, ok := err.(notFound)
	return ok && te.NotFoundError()
}

// IsInsufficientBalance is a helper method for determining if an error indicates insufficient balance
func IsInsufficientBalance(err error) bool {
	type insufficientBalance interface {
		InsufficientBalance() bool
	}
	te, ok := err.(insufficientBalance)
	return ok && te.InsufficientBalance()
}

// IsUnauthorized is a helper method for determining if an error indicates the wallet unauthorized
func IsUnauthorized(err error) bool {
	type unauthorized interface {
		Unauthorized() bool
	}
	te, ok := err.(unauthorized)
	return ok && te.Unauthorized()
}

// IsInvalidSignature is a helper method for determining if an error indicates there was an invalid signature
func IsInvalidSignature(err error) bool {
	type invalidSignature interface {
		InvalidSignature() bool
	}
	te, ok := err.(invalidSignature)
	return ok && te.InvalidSignature()
}

// AlreadyExists is a helper method for determining if an error indicates the resource already exists
func AlreadyExists(err error) bool {
	type alreadyExists interface {
		AlreadyExistsError() bool
	}
	te, ok := err.(alreadyExists)
	return ok && te.AlreadyExistsError()
}
