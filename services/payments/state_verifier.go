package payments

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/payments"
)

// A list of hex encoded previously valid PCR2 values
var previousPCR2Values = []string{
	// 2023-10-13 Releases
	"9626c96c05dfefc0988d7150f5741481f6adeddade120c3542364f0a98f05f4e2b2b0ba5b4b54fa1deaa5ebd3e64eeee",
	"c8c1b556be8c0e6ba2cb6c7e86eb621d7289afcbf4796910afb5b694d844204b8e3ba53f4d16024aacca8b753327285d",
	"f128a8cbcf58ec832c1b95ca50fac6229d780c7d991e901433946c96c06c57e3d26ace50adfefefcdf9e7539df3cf8aa",
	"e0c0d819451abc2224a62ea1791fc813ad0b6c7bb0d1ff9700c954939691f619b3ab231b9534acdd14e666b121d1012c",
	// 2023-11-09 Release
	"cbf60e4ebe608c3785f35bd990dba5d77d273a38ebe6aafde99190346b9f5eb842db973e854bf8e002a4e12de3f620f4",
	"4cd91f7f6e0585bcdeb91bf0760456b5c5199f377927cc9340fdceac6aac5ac99509ca6c495a56e8a646dc9269ef0af4",
	// 2023-11-14 Release
	"a3cf13175572a6ccc4568be4b8459217ce821d2c11756fe3b0224075a8ddfb9b61bf56d2af873dc905480e6367448482",
}

type VerifierStore struct {
	verifiers map[string]nitro.Verifier
}

func NewVerifierStore() (*VerifierStore, error) {
	pcrs, err := nitro.GetPCRs()
	if err != nil {
		return nil, errors.New("could not retrieve nitro PCRs")
	}

	s := VerifierStore{verifiers: map[string]nitro.Verifier{}}

	// always accept attestations matching our own
	pubKey := hex.EncodeToString(pcrs[2])
	s.verifiers[pubKey] = nitro.NewVerifier(map[uint][]byte{
		0: pcrs[0],
		1: pcrs[1],
		2: pcrs[2],
	})

	for _, pcr2Hex := range previousPCR2Values {
		pcr2, err := hex.DecodeString(pcr2Hex)
		if err != nil {
			return nil, errors.New("could not decode previous PCRs")
		}
		s.verifiers[pcr2Hex] = nitro.NewVerifier(map[uint][]byte{
			1: nitro.ExpectedPCR1,
			2: pcr2,
		})
	}

	return &s, nil

}

func (s *VerifierStore) LookupVerifier(ctx context.Context, keyID string, updatedAt time.Time) (context.Context, *payments.Verifier, error) {
	for k, v := range s.verifiers {
		if k == keyID {
			v.Now = func() time.Time { return updatedAt }
			vv := (payments.Verifier)(v)
			return ctx, &vv, nil
		}
	}
	return ctx, nil, fmt.Errorf("unknown key: %s", keyID)
}
