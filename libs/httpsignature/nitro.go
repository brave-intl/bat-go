package httpsignature

import (
	"github.com/brave-intl/bat-go/libs/nitro"
)

// NitroSigner is a placeholder struct to sign using a nitro attestation
type NitroSigner struct{ nitro.Signer }

// NitroVerifier specifies the PCR values required for verification
type NitroVerifier struct{ nitro.Verifier }

// NewNitroVerifier returns a new verifier for nitro attestations
func NewNitroVerifier(pcrs map[uint][]byte) NitroVerifier {
	return NitroVerifier{nitro.NewVerifier(pcrs)}
}
