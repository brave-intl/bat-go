package payments

import (
	"encoding/json"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PrepareTx Tx

// GetAmount returns the amount of the transaction
func (pt *PrepareTx) GetAmount() decimal.Decimal {
	return pt.Amount
}

// GetCustodian returns the custodian of the transaction
func (pt *PrepareTx) GetCustodian() Custodian {
	return pt.Custodian
}

// Tx - this is the tx going to prepare workers from report
type Tx struct {
	To        string          `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	ID        string          `json:"idempotencyKey"`
	Custodian Custodian       `json:"custodian"`
}

type isTransaction interface {
	GetCustodian() Custodian
	GetAmount() decimal.Decimal
}

// GetCustodian returns the custodian of the transaction
func (t *Tx) GetCustodian() Custodian {
	return t.Custodian
}

// GetAmount returns the amount of the transaction
func (t *Tx) GetAmount() decimal.Decimal {
	return t.Amount
}

// MarshalJSON implements custom json marshaling (output json naming differences) for Tx
func (t *Tx) MarshalJSON() ([]byte, error) {
	type TxAlias Tx
	return json.Marshal(&struct {
		*TxAlias
	}{
		TxAlias: (*TxAlias)(t),
	})
}

// UnmarshalJSON implements custom json unmarshaling (convert altcurrency) for Tx
func (t *PrepareTx) UnmarshalJSON(data []byte) error {
	type TxAlias Tx
	aux := &struct {
		*TxAlias
		To        string          `json:"address"`
		Amount    decimal.Decimal `json:"probi"`
		Publisher string          `json:"publisher"`
		BatchID   string          `json:"transactionId"`
		Custodian string          `json:"walletProvider"`
	}{
		TxAlias: (*TxAlias)(t),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	t.Amount = altcurrency.BAT.FromProbi(aux.Amount)
	t.To = aux.To
	t.Custodian = Custodian(aux.Custodian)

	// uuidv5 with settlement namespace to get the idemptotency key for this publisher/transactionId
	// transactionId is the settlement batch identifier, and publisher is the identifier of the recipient
	t.ID = uuid.NewSHA1(
		idempotencyNamespace, []byte(aux.BatchID+aux.Publisher)).String()

	return nil
}

// An AttestedTx is a transaction that has been attested by an enclave
type AttestedTx struct {
	Tx
	Version             string `json:"version"`
	State               string `json:"state"`
	DocumentID          string `json:"documentId"`
	AttestationDocument string `json:"attestationDocument"` // base64 encoded
}

// UnmarshalJSON implements custom json unmarshaling
func (at *AttestedTx) UnmarshalJSON(data []byte) error {
	type AttestedTxAlias AttestedTx
	aux := &AttestedTxAlias{}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	at.Amount = aux.Amount
	at.To = aux.To
	at.Custodian = Custodian(aux.Custodian)
	at.ID = aux.ID
	at.Version = aux.Version
	at.State = aux.State
	at.AttestationDocument = aux.AttestationDocument

	return nil
}
