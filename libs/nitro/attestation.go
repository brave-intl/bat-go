package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/hf/nitrite"
	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
)

// RootAWSNitroCert is the root certificate for the nitro enclaves in aws,
// retrieved from https://aws-nitro-enclaves.amazonaws.com/AWS_NitroEnclaves_Root-G1.zip
var RootAWSNitroCert = `-----BEGIN CERTIFICATE-----
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

// Attest takes as input a nonce, user-provided data and a public key, and then
// asks the Nitro hypervisor to return a signed attestation document that
// contains all three values.
func Attest(ctx context.Context, nonce, userData, publicKey []byte) ([]byte, error) {

	var logger = logging.Logger(ctx, "nitro.Attest")
	s, err := nsm.OpenDefaultSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = s.Close(); err != nil {
			logger.Error().Msgf("Attestation: Failed to close default NSM session: %s", err)
		}
	}()

	res, err := s.Send(&request.Attestation{
		Nonce:     nonce,
		UserData:  userData,
		PublicKey: publicKey,
	})
	if err != nil {
		return nil, err
	}

	if res.Attestation == nil || res.Attestation.Document == nil {
		return nil, errors.New("NSM device did not return an attestation")
	}

	return res.Attestation.Document, nil
}

// Signer is a placeholder struct to sign using a nitro attestation
type Signer struct{}

// Sign the message using the nitro signer
func (s Signer) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	return Attest(context.Background(), nil, message, nil)
}

// Verifier specifies the PCR values required for verification
type Verifier struct {
	PCRs map[uint][]byte
	now  func() time.Time
}

// NewVerifier returns a new verifier for nitro attestations
func NewVerifier(pcrs map[uint][]byte) Verifier {
	return Verifier{
		PCRs: pcrs,
		now:  time.Now,
	}
}

// Verify the signature sig for message using the nitro verifier
func (v Verifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM([]byte(RootAWSNitroCert))
	if !ok {
		return false, errors.New("could not create a valid root cert pool")
	}

	res, err := nitrite.Verify(
		sig,
		nitrite.VerifyOptions{
			Roots:       pool,
			CurrentTime: v.now(),
		},
	)
	if nil != err {
		return false, err
	}

	if !bytes.Equal(res.Document.UserData, message) {
		return false, nil
	}

	if len(v.PCRs) == 0 {
		return false, nil
	}

	for pcr, expectedV := range v.PCRs {
		v, exists := res.Document.PCRs[pcr]
		if !exists {
			return false, nil
		}
		if !bytes.Equal(expectedV, v) {
			return false, nil
		}
	}
	return true, nil
}

// String returns the stringified PCR values we are checking against
func (v Verifier) String() string {
	return fmt.Sprint(v.PCRs)
}
