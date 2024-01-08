// Package wallet defines common datastructures and an interface for cryptocurrency wallets
package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/btcsuite/btcutil/base58"
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

type SolanaLinkReq struct {
	Pub   string
	Sig   string
	Msg   string
	Nonce string
}

func (w *Info) LinkSolanaAddress(_ context.Context, s SolanaLinkReq) error {
	if err := w.isLinkReqValid(s); err != nil {
		return newLinkSolanaAddressError(err)
	}

	w.UserDepositDestination = s.Pub

	return nil
}

func (w *Info) isLinkReqValid(s SolanaLinkReq) error {
	if err := verifySolanaSignature(s.Pub, s.Msg, s.Sig); err != nil {
		return err
	}

	p := newSolMsgParser(w.ID, s.Pub, s.Nonce)
	rm, err := p.parse(s.Msg)
	if err != nil {
		return fmt.Errorf("parsing error: %w", err)
	}

	if err := verifyRewardsSignature(w.PublicKey, rm.msg, rm.sig); err != nil {
		return err
	}

	return nil
}

const publicKeyLength = 32

const (
	errBadPublicKeyLength     Error = "wallet: bad public key length"
	errInvalidSolanaSignature Error = "wallet: invalid solana signature for message and public key"
)

func verifySolanaSignature(pub, msg, sig string) error {
	b := base58.Decode(pub)
	if len(b) != publicKeyLength {
		return fmt.Errorf("error verifying solana signature: %w", errBadPublicKeyLength)
	}
	pubKey := ed25519.PublicKey(b)

	decSig, err := base64.URLEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("error decoding solana signature: %w", err)
	}

	if !ed25519.Verify(pubKey, []byte(msg), decSig) {
		return errInvalidSolanaSignature
	}

	return nil
}

type rewMsg struct {
	msg string
	sig string
}

const (
	errInvalidPartsLineBreak Error = "wallet: invalid number of lines"
	errInvalidPartsColon     Error = "wallet: invalid parts colon"
	errInvalidParts          Error = "wallet: invalid number of parts"
	errInvalidPaymentID      Error = "wallet: payment id does not match"
	errInvalidSolanaPubKey   Error = "wallet: solana public key does not match"
	errInvalidRewardsMessage Error = "wallet: invalid rewards message"
)

type solMsgParser struct {
	paymentID string
	solPub    string
	nonce     string
}

func newSolMsgParser(paymentID, solPub, nonce string) solMsgParser {
	return solMsgParser{
		paymentID: paymentID,
		solPub:    solPub,
		nonce:     nonce,
	}
}

// parse a linking message and return the rewards part.
// For a message to parse successfully it must be a valid format and successfully match the
// configured parser parameters paymentID, solPub and nonce.
//
// The message format should be three lines with a colon delimiter. For example,
// <some-text>:<rewards-payment-id>
// <some-text>:<solana-address>
// <some-text>:<rewards-payment-id>.<nonce>.<rewardsSignature>
func (s solMsgParser) parse(msg string) (rewMsg, error) {
	n := strings.Split(msg, "\n")
	if len(n) != 3 {
		return rewMsg{}, errInvalidPartsLineBreak
	}

	var parts []string
	for i := range n {
		p := strings.Split(n[i], ":")
		if len(p) != 2 {
			return rewMsg{}, errInvalidPartsColon
		}
		parts = append(parts, p[1])
	}

	if len(parts) != 3 {
		return rewMsg{}, errInvalidParts
	}

	if parts[0] != s.paymentID {
		return rewMsg{}, errInvalidPaymentID
	}

	if parts[1] != s.solPub {
		return rewMsg{}, errInvalidSolanaPubKey
	}

	exp := s.paymentID + "." + s.nonce + "."
	for i := range exp {
		if parts[2][i] != exp[i] {
			return rewMsg{}, errInvalidRewardsMessage
		}
	}

	rm := rewMsg{
		msg: parts[2][:len(exp)-1], // -1 removes the trailing .
		sig: parts[2][len(exp):],
	}

	return rm, nil
}

const errInvalidRewardsSignature Error = "wallet: invalid rewards signature for message and public key"

func verifyRewardsSignature(pub, msg, sig string) error {
	b, err := hex.DecodeString(pub)
	if err != nil {
		return fmt.Errorf("error decoding rewards public key: %w", err)
	}

	if len(b) != publicKeyLength {
		return fmt.Errorf("error verifying rewards signature: %w", errBadPublicKeyLength)
	}
	pubKey := ed25519.PublicKey(b)

	decSig, err := base64.URLEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("error decoding rewards signature: %w", err)
	}

	if !ed25519.Verify(pubKey, []byte(msg), decSig) {
		return errInvalidRewardsSignature
	}

	return nil
}

type LinkSolanaAddressError struct {
	err error
}

func newLinkSolanaAddressError(err error) error {
	return &LinkSolanaAddressError{err: err}
}

func (e *LinkSolanaAddressError) Error() string {
	return e.err.Error()
}

func (e *LinkSolanaAddressError) Unwrap() error {
	return e.err
}

type Error string

func (e Error) Error() string {
	return string(e)
}
