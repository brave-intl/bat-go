package custodian

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/shengdoushi/base58"
	"github.com/shopspring/decimal"
)

// Transaction describes a payout transaction from the settlement wallet to a publisher
type Transaction struct {
	AltCurrency      *altcurrency.AltCurrency `json:"altcurrency"`
	Authority        string                   `json:"authority"`
	Amount           decimal.Decimal          `json:"amount"`
	ExchangeFee      decimal.Decimal          `json:"commission"`
	FailureReason    string                   `json:"failureReason,omitempty"`
	Currency         string                   `json:"currency"`
	Destination      string                   `json:"address"`
	Publisher        string                   `json:"owner"`
	BATPlatformFee   decimal.Decimal          `json:"fees"`
	Probi            decimal.Decimal          `json:"probi"`
	ProviderID       string                   `json:"hash"`
	WalletProvider   string                   `json:"walletProvider"`
	WalletProviderID string                   `json:"walletProviderId"`
	Channel          string                   `json:"publisher"`
	SignedTx         string                   `json:"signedTx"`
	Status           string                   `json:"status"`
	SettlementID     string                   `json:"transactionId" valid:"uuidv4"`
	TransferFee      decimal.Decimal          `json:"fee"`
	Type             string                   `json:"type"`
	ValidUntil       time.Time                `json:"validUntil,omitempty"`
	DocumentID       string                   `json:"documentId,omitempty"`
	Note             string                   `json:"note"`
}

// TransferID generate the transfer id
func (tx Transaction) TransferID() string {
	inputs := []string{
		tx.SettlementID,
		tx.Destination,
		tx.Channel,
	}
	key := strings.Join(inputs, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// BitflyerTransferID generate the bitflier transfer id
func (tx Transaction) BitflyerTransferID() string {
	inputs := []string{
		tx.SettlementID,
		tx.WalletProviderID,
	}
	key := strings.Join(inputs, "_")
	bytes := sha256.Sum256([]byte(key))
	refID := base58.Encode(bytes[:], base58.IPFSAlphabet)
	return refID
}

// Log logs a message
func (tx Transaction) Log() {
	fmt.Println(tx.Destination, tx.Publisher, tx.TransferID(), tx.Channel)
}

// IsProcessing returns true if the transaction status is processing
func (tx Transaction) IsProcessing() bool {
	return tx.Status == "processing"
}

// IsFailed returns true if the transaction status is failed
func (tx Transaction) IsFailed() bool {
	return tx.Status == "failed"
}

// IsComplete returns true if the transaction status is completed
func (tx Transaction) IsComplete() bool {
	return tx.Status == "completed"
}
