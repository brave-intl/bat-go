package nitro

import (
	"bytes"
	"context"
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/hf/nitrite"
	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
)

// per https://docs.aws.amazon.com/enclaves/latest/user/set-up-attestation.html
// PCR1 is a contiguous measurement of the kernel and boot ramfs data.
// the kernel and boot ramfs are present in /usr/share/nitro_enclaves/blobs/
// and shipped as part of the official nitro cli tooling. as a result, they
// do not vary based on the docker image we provide and are consistent across
// all images that we produce. for simplicity, we hardcode the current PCR1 value
// corresponding to the latest nitro cli tooling release.
// NOTE: this will need to be changed if the nitro cli tools are updated
const pcr1Hex = "dc9f5af64d83079f2fddca94016f1cba17eb95eb78638eaff32c75517274f05537aabfcbe8e02cb8837906197cf58506"

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

// ExpectedPCR1 is the expected PCR1 value for all images built with the official nitro cli tooling
var ExpectedPCR1 []byte

func init() {
	var err error
	ExpectedPCR1, err = hex.DecodeString(pcr1Hex)
	if err != nil {
		log.Fatal(err)
	}
}

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

// GetPCRs returns the PCR values for the currently running enclave by
// performing an attestation and parsing the result
func GetPCRs() (map[uint][]byte, error) {
	sig, err := Attest(context.Background(), nil, nil, nil)
	if err != nil {
		return nil, err
	}
	verifier := NewVerifier(nil)
	_, pcrs, err := verifier.verifySigOnlyNotPCRs(nil, sig, crypto.Hash(0))
	return pcrs, err
}

// Signer is a placeholder struct to sign using a nitro attestation
type Signer struct{}

// Sign the message using the nitro signer
func (s Signer) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	hash := sha256.Sum256(message)
	return Attest(context.Background(), nil, hash[:], nil)
}

// Verifier specifies the PCR values required for verification
type Verifier struct {
	PCRs map[uint][]byte
	Now  func() time.Time
}

// NewVerifier returns a new verifier for nitro attestations
func NewVerifier(pcrs map[uint][]byte) Verifier {
	return Verifier{
		PCRs: pcrs,
		Now:  time.Now,
	}
}

// verifySigOnlyNotPCRs verifies that a signature is good but does not check the PCR values
func (v Verifier) verifySigOnlyNotPCRs(message, sig []byte, opts crypto.SignerOpts) (bool, map[uint][]byte, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM([]byte(RootAWSNitroCert))
	if !ok {
		return false, nil, errors.New("could not create a valid root cert pool")
	}

	res, err := nitrite.Verify(
		sig,
		nitrite.VerifyOptions{
			Roots:       pool,
			CurrentTime: v.Now(),
		},
	)
	if nil != err {
		return false, nil, err
	}

	var userdata []byte
	if message != nil {
		hash := sha256.Sum256(message)
		userdata = hash[:]
	}
	if !bytes.Equal(res.Document.UserData, userdata) {
		return false, nil, nil
	}

	return true, res.Document.PCRs, nil
}

// Verify the signature sig for message using the nitro verifier
func (v Verifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	valid, pcrs, err := v.verifySigOnlyNotPCRs(message, sig, opts)
	if err != nil || !valid {
		fmt.Println("ERR: verify failed or not valid")
		return valid, err
	}

	if len(v.PCRs) == 0 {
		fmt.Println("ERR: pcrs was empty")
		return false, nil
	}

	for pcr, expectedV := range v.PCRs {
		v, exists := pcrs[pcr]
		if !exists {
			fmt.Println("ERR: pcr was missing", pcr)
			return false, nil
		}
		if !bytes.Equal(expectedV, v) {
			fmt.Println("ERR: pcr did not match", pcr)
			return false, nil
		}
	}
	return true, nil
}

// String returns the stringified PCR values we are checking against
func (v Verifier) String() string {
	return fmt.Sprint(v.PCRs)
}
