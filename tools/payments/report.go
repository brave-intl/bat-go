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
	"fmt"
	"io"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/shopspring/decimal"
	nitrodoc "github.com/veracruz-project/go-nitro-enclave-attestation-document"
)

// AttestedReport is the report of payouts after being prepared
type AttestedReport []*AttestedTx

// SumBAT sums the total amount of BAT in the report.
func (ar AttestedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range ar {
		total = total.Add(v.GetAmount())
	}
	return total
}

// PreparedReport is the report of payouts prior to being prepared
type PreparedReport []*PrepareTx

// SumBAT sums the total amount of BAT in the report.
func (r PreparedReport) SumBAT() decimal.Decimal {
	total := decimal.Zero
	for _, v := range r {
		total = total.Add(v.GetAmount())
	}
	return total
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
var rootAWSNitroCert = `-----BEGIN CERTIFICATE-----
MIICETCCAZagAwIBAgIRAPkxdWgbkK/hHUbMtOTn+FYwCgYIKoZIzj0EAwMwSTEL
MAkGA1UEBhMCVVMxDzANBgNVBAoMBkFtYXpvbjEMMAoGA1UECwwDQVdTMRswGQYD
VQQDDBJhd3Mubml0cm8tZW5jbGF2ZXMwHhcNMTkxMDI4MTMyODA1WhcNNDkxMDI4
MTQyODA1WjBJMQswCQYDVQQGEwJVUzEPMA0GA1UECgwGQW1hem9uMQwwCgYDVQQL
DANBV1MxGzAZBgNVBAMMEmF3cy5uaXRyby1lbmNsYXZlczB2MBAGByqGSM49AgEG
BSuBBAAiA2IABPwCVOumCMHzaHDimtqQvkY4MpJzbolL//Zy2YlES1BR5TSksfbb
48C8WBoyt7F2Bw7eEtaaP+ohG2bnUs990d0JX28TcPQXCEPZ3BABIeTPYwEoCWZE
h8l5YoQwTcU/9KNCMEAwDwYDVR0TAQH/BAUwAwEB/zAdBgNVHQ4EFgQUkCW1DdkF
R+eWw5b6cp3PmanfS5YwDgYDVR0PAQH/BAQDAgGGMAoGCCqGSM49BAMDA2kAMGYC
MQCjfy+Rocm9Xue4YnwWmNJVA44fA0P5W2OpYow9OYCVRaEevL8uO1XYru5xtMPW
rfMCMQCi85sWBbJwKKXdS6BptQFuZbT73o/gBh1qUxl/nNr12UO8Yfwr6wPLb+6N
IwLz3/Y=
-----END CERTIFICATE-----`

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

// Compare takes a prepared report and validates the transactions are the same as the attested report
func Compare(pr PreparedReport, ar AttestedReport) error {
	// check that the number of transactions match
	if len(pr) != len(ar) {
		return fmt.Errorf("number of transactions do not match - attested: %d; prepared: %d", len(ar), len(pr))
	}

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
