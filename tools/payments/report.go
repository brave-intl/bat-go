package payments

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/shopspring/decimal"
	nitrodoc "github.com/veracruz-project/go-nitro-enclave-attestation-document"
)

var (
	// ErrDuplicateDepositDestination indicates that a report contains duplicate deposit destinations.
	ErrDuplicateDepositDestination = errors.New("duplicate deposit destination")
)

// AttestedReport is the report of payouts after being prepared
type AttestedReport []*payments.AttestedTx

// SumBAT sums the total amount of BAT in the report.
func (ar AttestedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range ar {
		total = total.Add(v.GetAmount())
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

// PreparedReport is the report of payouts prior to being prepared
type PreparedReport []*payments.PrepareTx

// SumBAT sums the total amount of BAT in the report.
func (r PreparedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range r {
		total = total.Add(v.GetAmount())
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

// ReadReport reads a report from the reader
func ReadReport(report any, reader io.Reader) error {
	if err := json.NewDecoder(reader).Decode(report); err != nil {
		return fmt.Errorf("failed to parse report: %w", err)
	}
	return nil
}

// rootAWSNitroCert is the root certificate for the nitro enclaves in aws,
// retrieved from https://aws-nitro-enclaves.amazonaws.com/AWS_NitroEnclaves_Root-G1.zip
var rootAWSNitroCert = nitro.RootAWSNitroCert

// IsAttested allows the caller to validate if the transactions within the report are attested
func (ar AttestedReport) IsAttested() (bool, error) {
	// parse the root certificate
	block, _ := pem.Decode([]byte(rootAWSNitroCert))

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse certificate: %w", err)
	}

	for _, tx := range ar {
		// decode the attestation document base64
		doc, err := base64.StdEncoding.DecodeString(tx.AttestationDocument)
		if err != nil {
			return false, fmt.Errorf("failed to decode attestation document on tx: %s", tx.ID)
		}
		// authenticate the attestation document on the record
		document, err := nitrodoc.AuthenticateDocument(doc, *cert, true)
		if err != nil {
			return false, fmt.Errorf("failed to authenticate attestation document on tx: %s", tx.ID)
		}
		// authentically from nitro, now validate the signing bytes match
		if subtle.ConstantTimeCompare(
			[]byte(tx.DocumentID),
			document.User_Data) < 1 {
			return false, fmt.Errorf("attested userdata does not match document id: %s", tx.DocumentID)
		}
	}
	return true, nil
}

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

	return nil
}

// Submit performs a submission of approval from an operator to the settlement client
func (ar AttestedReport) Submit(ctx context.Context, key ed25519.PrivateKey, client SettlementClient) error {
	signer := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(key.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"host",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: key,
		Opts:     crypto.Hash(0),
	}

	return client.SubmitTransactions(ctx, signer, ar...)
}

// Prepare performs a preparation of transactions for a payout to the settlement client
func (r PreparedReport) Prepare(ctx context.Context, client SettlementClient) error {
	return client.PrepareTransactions(ctx, r...)
}
