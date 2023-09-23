package payments

import (
	"encoding/json"

	"github.com/brave-intl/bat-go/libs/altcurrency"
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
	Amount    decimal.Decimal `json:"amount"`
	To        string          `json:"to"`
	From      string          `json:"from"`
	Custodian Custodian       `json:"custodian"`
	PayoutID  string          `json:"payoutId"`
	Currency  string          `json:"currency"`
	DryRun    *string         `json:"dryRun"`
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
func (pt *PrepareTx) UnmarshalJSON(data []byte) error {
	type TxAlias Tx
	aux := &struct {
		*TxAlias
		To        string          `json:"address"`
		Amount    decimal.Decimal `json:"probi"`
		Publisher string          `json:"publisher"`
		BatchID   string          `json:"transactionId"`
		Custodian string          `json:"walletProvider"`
	}{
		TxAlias: (*TxAlias)(pt),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	pt.Amount = altcurrency.BAT.FromProbi(aux.Amount)
	pt.To = aux.To
	pt.Custodian = Custodian(aux.Custodian)
	pt.PayoutID = aux.BatchID

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
	at.Version = aux.Version
	at.State = aux.State
	at.DocumentID = aux.DocumentID
	at.AttestationDocument = aux.AttestationDocument
	at.DryRun = aux.DryRun

	return nil
}
