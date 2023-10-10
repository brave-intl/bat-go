package payments

import (
	"encoding/hex"
	"log"

	"github.com/brave-intl/bat-go/libs/httpsignature"
)

// this will need to be changed if the nitro cli tools are updated
const pcr1Hex = "dc9f5af64d83079f2fddca94016f1cba17eb95eb78638eaff32c75517274f05537aabfcbe8e02cb8837906197cf58506"

var pcr1 []byte

func init() {
	var err error
	pcr1, err = hex.DecodeString(pcr1Hex)
	if err != nil {
		log.Fatal(err)
	}
}

func NewNitroVerifier(pcr2Hex *string) (*httpsignature.SignatureParams, *httpsignature.NitroVerifier, error) {
	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{"digest"}

	pcrs := map[uint][]byte{
		1: pcr1,
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
