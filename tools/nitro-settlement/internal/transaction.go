package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	UpholdCustodian   = "uphold"
	GeminiCustodian   = "gemini"
	BitflyerCustodian = "bitflyer"
)

var (
	settlementIdempotencyNamespace, _ = uuid.Parse("1286fb9f-c6ac-4e97-97a3-9fd866c95926")
)

// ParseAttestedReport - take a filename and parse the transaction report
func ParseAttestedReport(ctx context.Context, reportLocation string) (*AttestedReport, error) {
	var report = new(AttestedReport)

	f, err := os.Open(reportLocation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse report: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(report); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}

	return report, nil
}

// ParseReport - take a filename and parse the transaction report
func ParseReport(ctx context.Context, reportLocation string) (*Report, error) {
	var report = new(Report)

	f, err := os.Open(reportLocation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse report: %w", err)
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(report); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}

	return report, nil
}

// AuthorizeTx - this is the tx going to authorize workers from attested report
type AuthorizeTx struct {
	To         string          `json:"to"`
	Amount     decimal.Decimal `json:"amount"`
	ID         string          `json:"idempotencyKey"`
	Custodian  string          `json:"custodian"`
	DocumentID string          `json:"documentId"`
	Signature  string          `json:"signature"`
}

// BuildSigningString - the string format that payments will sign over per tx
func (at AuthorizeTx) BuildSigningBytes() []byte {
	return []byte(fmt.Sprintf("%s|%s|%s|%s|%s",
		at.To, at.Amount.String(), at.ID, at.Custodian, at.DocumentID))
}

// PrepareTx - this is the tx going to prepare workers from report
type PrepareTx struct {
	To        string          `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	ID        string          `json:"idempotencyKey"`
	Custodian string          `json:"custodian"`
}

// MarshalJSON - custom json marshaling (output json naming differences)
func (pt *PrepareTx) MarshalJSON() ([]byte, error) {
	type PrepareTxAlias PrepareTx
	return json.Marshal(&struct {
		*PrepareTxAlias
	}{
		PrepareTxAlias: (*PrepareTxAlias)(pt),
	})
}

// UnmarshalJSON - custom json unmarshaling (convert altcurrency)
func (pt *PrepareTx) UnmarshalJSON(data []byte) error {
	type PrepareTxAlias PrepareTx
	aux := &struct {
		*PrepareTxAlias
		To        string          `json:"address"`
		Amount    decimal.Decimal `json:"probi"`
		Publisher string          `json:"publisher"`
		BatchID   string          `json:"transactionId"`
		Custodian string          `json:"walletProvider"`
	}{
		PrepareTxAlias: (*PrepareTxAlias)(pt),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	pt.Amount = altcurrency.BAT.FromProbi(aux.Amount)
	pt.To = aux.To
	pt.Custodian = aux.Custodian

	// uuidv5 with settlement namespace to get the idemptotency key for this publisher/transactionId
	// transactionId is the settlement batch identifier, and publisher is the identifier of the recipient
	pt.ID = uuid.NewSHA1(
		settlementIdempotencyNamespace, []byte(fmt.Sprintf("%s|%s", aux.BatchID, aux.Publisher))).String()

	return nil
}

// Report - the payout report of prepare transactions
type Report []PrepareTx

// SumBAT - sum up the amount of bat in the report
func (r Report) SumBAT(custodians ...string) decimal.Decimal {
	var total = decimal.Zero
	for i := 0; i < len([]PrepareTx(r)); i++ {
		if len(custodians) > 0 {
			for _, c := range custodians {
				if strings.EqualFold([]PrepareTx(r)[i].Custodian, c) {
					total = total.Add([]PrepareTx(r)[i].Amount)
				}
			}
		} else { // all custodians
			total = total.Add([]PrepareTx(r)[i].Amount)
		}
	}
	return total
}

// Length - report length
func (r Report) Length() int {
	return len([]PrepareTx(r))
}

// AttestedReport - the attested transactions
type AttestedReport []AuthorizeTx

// Length - report length
func (ar AttestedReport) Length() int {
	return len([]AuthorizeTx(ar))
}

// SumBAT - sum up the amount of bat in the report
func (ar AttestedReport) SumBAT(custodians ...string) decimal.Decimal {
	var total = decimal.Zero
	for i := 0; i < len([]AuthorizeTx(ar)); i++ {
		if len(custodians) > 0 {
			for _, c := range custodians {
				if strings.EqualFold([]AuthorizeTx(ar)[i].Custodian, c) {
					total = total.Add([]AuthorizeTx(ar)[i].Amount)
				}
			}
		} else { // all custodians
			total = total.Add([]AuthorizeTx(ar)[i].Amount)
		}
	}
	return total
}

// Verify - verify signatures on each of the records in the report
func (ar AttestedReport) Verify(ctx context.Context, pub ed25519.PublicKey) error {
	for i := 0; i < len([]AuthorizeTx(ar)); i++ { // for every transaction
		msg := []AuthorizeTx(ar)[i].BuildSigningBytes()
		sig, err := hex.DecodeString([]AuthorizeTx(ar)[i].Signature)
		if err != nil {
			return LogAndError(ctx, fmt.Errorf("tx invalid sig # %d - %s - %s",
				i, msg, sig,
			), "Verify", "could not decode transaction signature")
		}

		if !ed25519.Verify(pub, msg, sig) {
			// failed verification
			return LogAndError(ctx, fmt.Errorf("tx invalid sig # %d - %s - %s",
				i, msg, sig,
			), "Verify", "could not verify transaction signature")
		}
	}
	return nil
}
