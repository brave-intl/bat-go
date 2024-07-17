package payments

import (
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/nitro"
)

// NewNitroVerifier provides a verifier for a given PRC2 string that is used to verify the signature
// in an HTTP signed response
func NewNitroVerifier(pcr2Hex *string) (*httpsignature.SignatureParams, nitro.Verifier, error) {
	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{
		httpsignature.DigestHeader,
		httpsignature.DateHeader,
	}

	pcr2 := *pcr2Hex
	if pcr2 == "" {
		const env = "NITRO_EXPECTED_PCR2"
		pcr2 = os.Getenv(env)
		if pcr2 == "" {
			return nil, nitro.Verifier{}, fmt.Errorf(
				"PCR2 value must be given either on the command line or in the %s environment variable",
				env,
			)
		}
	}
	verifier, err := nitro.NewVerifier(pcr2)
	if err != nil {
		return nil, nitro.Verifier{}, err
	}

	return &sp, verifier, nil
}
