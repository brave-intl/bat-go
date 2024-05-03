package payments

import (
	"bufio"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/shopspring/decimal"
)

var (
	// ErrDuplicateDepositDestination indicates that a report contains duplicate deposit destinations.
	ErrDuplicateDepositDestination = errors.New("duplicate deposit destination")
	// ErrMismatchedDepositAmounts indicates that transaction that share a To do not share an Amount.
	ErrMismatchedDepositAmounts = errors.New("mismatched deposit amounts")
)

// AttestedReport is the report of payouts after being prepared
type AttestedReport []payments.PrepareResponse

// SumBAT sums the total amount of BAT in the report.
func (ar AttestedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range ar {
		// FIXME assumes BAT
		total = total.Add(v.Amount)
	}
	return total
}

func (r AttestedReport) EnsureUniqueDest() error {
	u := make(map[string]struct{})

	for _, tx := range r {
		if _, ok := u[tx.To]; ok {
			return fmt.Errorf(
				"error validating attested report duplicate to %s: %w",
				tx.To, ErrDuplicateDepositDestination,
			)
		}

		u[tx.To] = struct{}{}
	}

	return nil
}

// EnsureTransactionAmountsMatch checks each transaction by To address and errors if the Amounts do
// not match between the AttestedReport and the provided PreparedReport.
func (ar AttestedReport) EnsureTransactionAmountsMatch(pr PreparedReport) error {
	preparedMap := make(map[string]decimal.Decimal, len(pr))
	for _, paymentDetails := range pr {
		preparedMap[paymentDetails.To] = paymentDetails.Amount
	}
	for _, attestedDetails := range ar {
		if !preparedMap[attestedDetails.To].Equal(attestedDetails.Amount) {
			return fmt.Errorf(
				"%w for %s - prepared: %s, attested: %s",
				ErrMismatchedDepositAmounts,
				attestedDetails.To,
				preparedMap[attestedDetails.To],
				attestedDetails.Amount,
			)
		}
	}
	return nil
}

func (r AttestedReport) Validate() error {
	for _, reportEntry := range r {
		if _, err := govalidator.ValidateStruct(reportEntry); err != nil {
			return fmt.Errorf("failed to validate reportEntry: %w", err)
		}
	}
	return r.EnsureUniqueDest()
}

// PreparedReport is the report of payouts prior to being prepared
type PreparedReport []*payments.PaymentDetails

// SumBAT sums the total amount of BAT in the report.
func (r PreparedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range r {
		// FIXME assumes BAT
		total = total.Add(v.Amount)
	}
	return total
}

func (r PreparedReport) EnsureUniqueDest() error {
	u := make(map[string]struct{})

	for _, tx := range r {
		if _, ok := u[tx.To]; ok {
			return fmt.Errorf(
				"error validating prepare report duplicate to %s: %w",
				tx.To, ErrDuplicateDepositDestination,
			)
		}

		u[tx.To] = struct{}{}
	}

	return nil
}

func (r PreparedReport) Validate() error {
	for _, reportEntry := range r {
		if _, err := govalidator.ValidateStruct(reportEntry); err != nil {
			return fmt.Errorf("failed to validate reportEntry: %w", err)
		}
	}
	return r.EnsureUniqueDest()
}

// ReadReport reads a report from the reader
func ReadReport(report any, reader io.Reader) error {
	if err := json.NewDecoder(reader).Decode(report); err != nil {
		return fmt.Errorf("failed to parse report: %w", err)
	}
	return nil
}

// ReadReportFromResponses reads a report from the reader
func ReadReportFromResponses(report *AttestedReport, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	tmp := make(map[string]payments.PrepareResponse)
	for scanner.Scan() {
		var resp payments.PrepareResponse
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			return err
		}
		// dedupe by idempotency key
		tmp[resp.IdempotencyKey().String()] = resp
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, resp := range tmp {
		*report = append(*report, resp)
	}
	return nil
}

// rootAWSNitroCert is the root certificate for the nitro enclaves in aws,
// retrieved from https://aws-nitro-enclaves.amazonaws.com/AWS_NitroEnclaves_Root-G1.zip
var rootAWSNitroCert = nitro.RootAWSNitroCert

// Compare takes a prepared and attested report and validates that both contain the same number of transactions,
// that there is only a single deposit destination per transaction and that the total sum of BAT is the
// same in each report.
func Compare(pr PreparedReport, ar AttestedReport) error {
	// check that the number of transactions match
	if len(pr) != len(ar) {
		return fmt.Errorf("number of transactions do not match - attested: %d; prepared: %d", len(ar), len(pr))
	}

	// Check for duplicate deposit destinations in prepared report.
	if err := pr.EnsureUniqueDest(); err != nil {
		return err
	}

	// Check for duplicate deposit destinations in attested report.
	if err := ar.EnsureUniqueDest(); err != nil {
		return err
	}

	// Assert the total bat in each report is equal.
	p := pr.SumBAT()
	a := ar.SumBAT()

	if !p.Equal(a) {
		return fmt.Errorf("sum of BAT do not match - prepared: %s; attested: %s", p.String(), a.String())
	}

	// Check for individual transaction amounts that don't match between reports
	return ar.EnsureTransactionAmountsMatch(pr)
}

// Submit performs a submission of approval from an operator to the settlement client
func (ar AttestedReport) Submit(ctx context.Context, key ed25519.PrivateKey, client SettlementClient) error {
	signer := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(key.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: key,
		Opts:     crypto.Hash(0),
	}

	reqs := make([]payments.SubmitRequest, len(ar))
	for i, resp := range ar {
		reqs[i].DocumentID = resp.DocumentID
		reqs[i].PayoutID = resp.PayoutID
	}

	return client.SubmitTransactions(ctx, signer, reqs...)
}

// Prepare performs a preparation of transactions for a payout to the settlement client
func (r PreparedReport) Prepare(ctx context.Context, key ed25519.PrivateKey, client SettlementClient) error {
	signer := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(key.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: key,
		Opts:     crypto.Hash(0),
	}

	reqs := make([]payments.PrepareRequest, len(r))
	for i, paymentDetails := range r {
		reqs[i].PaymentDetails = *paymentDetails
	}

	return client.PrepareTransactions(ctx, signer, reqs...)
}

// OperatorKeys represents a file used for vault creation and approval that contains a set of
// operator keys mapped to names.
type OperatorKeys struct {
	Keys []payments.OperatorDataRequest `json:"keys"`
}
