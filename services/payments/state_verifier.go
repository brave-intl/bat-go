package payments

import (
	"bytes"
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
	// The map from hex value of PCR2 to the corresponding verifier. As we
	// hard-code PCR1 value in the executable, there can only be one PCR1 for
	// each PCR2 so we can use nitro.Verifier checking both PC1 and PCR2 as a
	// value here.
	allowedPCR2 map[string]nitro.Verifier
}

func NewVerifierStore() (*VerifierStore, error) {
	s := &VerifierStore{allowedPCR2: make(map[string]nitro.Verifier)}

	// always accept attestations matching our own
	currentPCRs, err := nitro.GetPCRs()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(currentPCRs[1], nitro.ExpectedPCR1) {
		return nil, errors.New("unexpected value of PCR1, perhaps Nitro kernel/initd were updated")
	}

	// always accept attestations matching our own
	pubKey := hex.EncodeToString(currentPCRs[2])
	s.allowedPCR2[pubKey] = nitro.Verifier{
		PCRs: nitro.PCRMap{
			0: currentPCRs[0],
			1: currentPCRs[1],
			2: currentPCRs[2],
		},
	}

	priorPCRs := strings.Split(os.Getenv("PRIOR_PCRS"), ",")

	// TODO: support multiple old PCR1, rather than using a hard-coded PCR1 for
	// all old PCRs.
	for i, pcr2Hex := range priorPCRs {
		v, err := nitro.NewVerifier(pcr2Hex)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid value in PRIOR_PCRS at index %d - %w", i, err)
		}
		s.allowedPCR2[pcr2Hex] = v
	}

	return s, nil
}

func (s *VerifierStore) LookupVerifier(ctx context.Context, keyID string, updatedAt time.Time) (context.Context, payments.Verifier, error) {
	verifier, exists := s.allowedPCR2[keyID]
	if !exists {
		return ctx, nil, fmt.Errorf("unknown key: %s", keyID)
	}
	return ctx, verifier, nil
}
