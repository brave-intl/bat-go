package httpsignature

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/hf/nitrite"
)

// NitroSigner is a placeholder struct to sign using a nitro attestation
type NitroSigner struct{}

// Sign the message using the nitro signer
func (s NitroSigner) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	return nitro.Attest(context.Background(), []byte{}, message, []byte{})
}

// NitroVerifier specifies the PCR values required for verification
type NitroVerifier struct {
	PCRs map[uint][]byte
	now  func() time.Time
}

func NewNitroVerifier(pcrs map[uint][]byte) NitroVerifier {
	return NitroVerifier{
		PCRs: pcrs,
		now:  time.Now,
	}
}

// Verify the signature sig for message using the nitro verifier
func (v NitroVerifier) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	pool := x509.NewCertPool()
	ok := pool.AppendCertsFromPEM([]byte(nitro.RootAWSNitroCert))
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
func (v NitroVerifier) String() string {
	return fmt.Sprint(v.PCRs)
}
