package payments

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/payments"
)

type VerifierStore struct {
	verifiers map[string]nitro.Verifier
}

type pcrList []string

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

	priorPCRs := strings.Split(os.Getenv("PRIOR_PCRS"), ",")

	for _, pcr2Hex := range priorPCRs {
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
