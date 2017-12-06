package wallet

import (
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/shopspring/decimal"
)

type WalletInfo struct {
	Id          string                   `json:"paymentId" valid:"uuidv4,optional"`
	Provider    string                   `json:"provider" valid:"in(uphold)"`
	ProviderId  string                   `json:"providerId" valid:"uuidv4"`
	AltCurrency *altcurrency.AltCurrency `json:"altcurrency" valid:"-"`
	PublicKey   string                   `json:"publicKey,omitempty" valid:"hexadecimal,optional"`
	LastBalance *Balance                 `json:"balances,omitempty" valid:"-"`
}

type TransactionInfo struct {
	Probi       decimal.Decimal          `json:"probi"`
	AltCurrency *altcurrency.AltCurrency `json:"altcurrency"`
	Destination string                   `json:"address"`
	Fee         decimal.Decimal          `json:"fee"`
	Status      string                   `json:"status"`
	ID          string                   `json:"id"`
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
	// NOTE VerifyTransaction must guard against transactions that seek to exploit parser differences
	// such as including additional fields that are not understood by local implementation but may
	// be understood by the upstream wallet provider.
	VerifyTransaction(transactionB64 string) (*TransactionInfo, error)
	SubmitTransaction(transactionB64 string, confirm bool) (*TransactionInfo, error)
	ConfirmTransaction(id string) (*TransactionInfo, error)
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
