package payments

import (
	"encoding/hex"
	"log"

	"github.com/brave-intl/bat-go/libs/httpsignature"
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
