package wallet

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

type WalletInfo struct {
	Id          string                  `json:"paymentId" valid:"uuidv4"`
	Provider    string                  `json:"provider" valid:"in(uphold)`
	ProviderId  string                  `json:"providerId"`
	AltCurrency altcurrency.AltCurrency `json:"altcurrency"`
	PublicKey   string                  `json:"publicKey,omitempty"`
	LastBalance Balance                 `json:"balances,omitempty"`
}

type TransactionInfo struct {
	Probi       decimal.Decimal
	AltCurrency altcurrency.AltCurrency
	Destination string
	// Status
	// Fees
	// Hash
}

type Balance struct {
	TotalProbi       decimal.Decimal
	SpendableProbi   decimal.Decimal
	ConfirmedProbi   decimal.Decimal
	UnconfirmedProbi decimal.Decimal
}

type Wallet interface {
	GetWalletInfo() WalletInfo
	Transfer(altcurrency altcurrency.AltCurrency, probi decimal.Decimal, destination string) (*TransactionInfo, error)
	VerifyTransaction(transactionB64 string) (*TransactionInfo, error)
	SubmitTransaction(transactionB64 string) (*TransactionInfo, error)
	GetBalance(refresh bool) (*Balance, error)
}

func IsInsufficientBalance(err error) bool {
	type insufficientBalance interface {
		InsufficientBalance() bool
	}
	te, ok := err.(insufficientBalance)
	return ok && te.InsufficientBalance()
}

func IsUnauthorized(err error) bool {
	type unauthorized interface {
		Unauthorized() bool
	}
	te, ok := err.(unauthorized)
	return ok && te.Unauthorized()
}

func IsInvalidSignature(err error) bool {
	type invalidSignature interface {
		InvalidSignature() bool
	}
	te, ok := err.(invalidSignature)
	return ok && te.InvalidSignature()
}
