package payments

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/payments"
)

// A list of hex encoded previously valid PCR2 values
var previousPCR2Values = []string{}

type VerifierStore struct {
	verifiers map[string]payments.Verifier
}

func NewVerifierStore() (*VerifierStore, error) {
	pcrs, err := nitro.GetPCRs()
	if err != nil {
		return nil, errors.New("could not retrieve nitro PCRs")
	}

	s := VerifierStore{}

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

func (s *VerifierStore) LookupVerifier(ctx context.Context, keyID string) (context.Context, *payments.Verifier, error) {
	for k, v := range s.verifiers {
		if k == keyID {
			return ctx, &v, nil
		}
	}
	return ctx, nil, fmt.Errorf("unknown key: %s", keyID)
}
