package payments

import (
	"encoding/hex"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/nitro"
)

func NewNitroVerifier(pcr2Hex *string) (*httpsignature.SignatureParams, *httpsignature.NitroVerifier, error) {
	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{"digest"}

	pcrs := map[uint][]byte{
		1: nitro.ExpectedPCR1,
	}

	if pcr2Hex != nil {
		pcr2, err := hex.DecodeString(*pcr2Hex)
		if err != nil {
			return nil, nil, err
		}
		pcrs[2] = pcr2
	}

	verifier := httpsignature.NewNitroVerifier(pcrs)
	return &sp, &verifier, nil
}
