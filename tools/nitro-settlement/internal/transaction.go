package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/shopspring/decimal"
)

const (
	UpholdCustodian   = "uphold"
	GeminiCustodian   = "gemini"
	BitflyerCustodian = "bitflyer"
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
		ID        string          `json:"transactionId"`
		Custodian string          `json:"walletProvider"`
	}{
		PrepareTxAlias: (*PrepareTxAlias)(pt),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	pt.Amount = altcurrency.BAT.FromProbi(aux.Amount)
	pt.To = aux.To
	pt.ID = aux.ID
	pt.Custodian = aux.Custodian
	return nil
}

type Report []PrepareTx

type AttestedReport []AuthorizeTx
