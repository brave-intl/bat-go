package payments

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Tx - this is the tx going to prepare workers from report
type Tx struct {
	To        string          `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	ID        string          `json:"idempotencyKey"`
	Custodian string          `json:"custodian"`
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
func (t *Tx) UnmarshalJSON(data []byte) error {
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
	t.Custodian = aux.Custodian

	// uuidv5 with settlement namespace to get the idemptotency key for this publisher/transactionId
	// transactionId is the settlement batch identifier, and publisher is the identifier of the recipient
	t.ID = uuid.NewSHA1(
		idempotencyNamespace, []byte(aux.BatchID+aux.Publisher)).String()

	return nil
}

// An AttestedTx is a transaction that has been attested by an enclave
type AttestedTx struct {
	Version             string          `json:"version"`
	To                  string          `json:"to"`
	Amount              decimal.Decimal `json:"amount"`
	ID                  string          `json:"idempotencyKey"`
	Custodian           Custodian       `json:"custodian"`
	State               string          `json:"state"`
	DocumentID          string          `json:"documentId"`
	AttestationDocument string          `json:"attestationDocument"` // base64 encoded
}

// DigestBytes returns a digest byte string which operators can sign over, and attestation
// userdata from nitro to be attested over
func (at AttestedTx) DigestBytes() []byte {
	return sha256.New().Sum([]byte(
		at.Version + at.To + at.Amount.String() + at.ID + at.Custodian.String() + at.DocumentID + at.State))[:]
}
