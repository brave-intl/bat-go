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
	"os"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/hf/nitrite"
	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
)

const PCRByteLength = 48

type PCRBytes = []byte

type PCRMap = map[uint]PCRBytes

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
const RootAWSNitroCert = `-----BEGIN CERTIFICATE-----
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

// ExpectedPCR1 is the expected PCR1 value for all images built with the
// official nitro cli tooling
var ExpectedPCR1 PCRBytes

func init() {
	var err error
	ExpectedPCR1, err = ParsePCRHex(pcr1Hex)
	if err != nil {
		panic(err)
	}
}

func ParsePCRHex(pcrHex string) (PCRBytes, error) {
	if pcrHex == "" {
		return nil, errors.New("PCR hex value cannot be an empty string")
	}
	pcr, err := hex.DecodeString(pcrHex)
	if err != nil {
		return nil, fmt.Errorf("PCR string value is not a valid hex - %w", err)
	}
	if len(pcr) != PCRByteLength {
		return nil, fmt.Errorf("unexpected length of PCR value - %d", len(pcr))
	}
	return pcr, nil
}

func hashMessage(message []byte) []byte {
	if len(message) == 0 {
		return nil
	}
	hash := sha256.Sum256(message)
	return hash[:]
}

// Attest takes as input a nonce, user-provided data and a public key, and then
// asks the Nitro hypervisor to return a signed attestation document that
// contains all three values.
func Attest(ctx context.Context, nonce, message, publicKey []byte) ([]byte, error) {
	messageHash := hashMessage(message)

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
		UserData:  messageHash,
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
var GetPCRs = sync.OnceValues(func() (PCRMap, error) {
	sig, err := Attest(context.Background(), nil, nil, nil)
	if err != nil {
		return nil, err
	}
	_, pcrs, err := verifySigOnlyNotPCRs(nil, sig, time.Now())
	return pcrs, err

})

// Sign the message using the nitro signer
func SignMessage(message []byte) (signature []byte, err error) {
	return Attest(context.Background(), nil, message, nil)
}

// A placeholder struct to sign using a nitro attestation
type Signer struct{}

func (s Signer) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	return SignMessage(message)
}

// verifySigOnlyNotPCRs verifies that a signature is good but does not check the PCR values
func verifySigOnlyNotPCRs(
	message []byte,
	sig []byte,
	verifyTime time.Time,
) (bool, PCRMap, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM([]byte(RootAWSNitroCert))
	if !ok {
		return false, nil, errors.New("could not create a valid root cert pool")
	}

	res, err := nitrite.Verify(
		sig,
		nitrite.VerifyOptions{
			Roots:       pool,
			CurrentTime: verifyTime,
		},
	)
	if nil != err {
		return false, nil, err
	}
	expectedHash := res.Document.UserData
	pcrs := res.Document.PCRs
	if len(pcrs) == 0 {
		return false, nil, errors.New("failed to get PCR for the Nitro-signed document")
	}

	messageHash := hashMessage(message)
	if !bytes.Equal(expectedHash, messageHash) {
		return false, pcrs, nil
	}

	return true, pcrs, nil
}

// Verifier specifies the PCR values required for verification
type Verifier struct {
	PCRs PCRMap
	// Check using the given time. zero time is equivalent to time.Now()
	VerificationTime time.Time
}

// Return a new verifier for nitro attestation that checks that messages were
// signed by a Nitro enclave with the given PCR2 value and the compile-time
// defined PCR1.
func NewVerifier(pcr2Hex string) (Verifier, error) {
	var v Verifier
	pcr2, err := ParsePCRHex(pcr2Hex)
	if err != nil {
		return v, nil
	}
	v.PCRs = make(PCRMap, 2)
	v.PCRs[1] = ExpectedPCR1
	v.PCRs[2] = pcr2
	return v, nil
}

// Verify the signature sig for message using the nitro verifier
func (v Verifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	if len(v.PCRs) == 0 {
		panic("PCRs must have at least one value")
	}
	t := v.VerificationTime
	if t.IsZero() {
		t = time.Now()
	}
	valid, pcrs, err := verifySigOnlyNotPCRs(message, sig, t)
	if err != nil {
		return false, err
	}

	// Log the reason for the validation failure.
	if !valid {
		fmt.Fprintf(os.Stderr, "ERR: Nitro attestation signature mismatch\n")
	}

	for pcr, expectedV := range v.PCRs {
		v, exists := pcrs[pcr]
		if !exists {
			fmt.Fprintf(os.Stderr, "ERR: pcr %d was missing\n", pcr)
			valid = false
		}
		if !bytes.Equal(expectedV, v) {
			fmt.Fprintf(os.Stderr, "ERR: pcr %d did not match\n", pcr)
			valid = false
		}
	}

	return valid, nil
}

// String returns the stringified PCR values we are checking against
func (v Verifier) String() string {
	return fmt.Sprint(v.PCRs)
}
