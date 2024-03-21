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
	// 2023-11-10 Release
	"3ab5f1d72bbee7bb3940843c3ca27e45f4b2d464f0adeb16bbc90266e0a97049bae4375310e7f8245f3cdc01a2d87adf",
	// 2023-11-13 Release
	"a3cf13175572a6ccc4568be4b8459217ce821d2c11756fe3b0224075a8ddfb9b61bf56d2af873dc905480e6367448482",
	// 2023-11-14 Release
	"69fb22bd080e5ff27add01c308ce1aa8294d6944d4df16e4d8bbefca8e154918623845fd34ff92bab7830175623fc0b8",
	// 2023-12-09 Release
	"90033743bba1fc16e70e69ceaedbe589c1449a9ed70de3d094e98c1ca18083eaa97b49389af48a0d074b5a3c1029e797",
	// 2023-12-11_1 Release
	"6c2cd8fd45ffd5a024f07b8885c7811ff74d291e3cbc2f6bc7c3925824408e7a1f24c3133f96e44ad8b1b925fead0767",
	// 2024-03-06 Release
	"31dcd0477a7f06736b14b0d7095c8c234973c2c19531750e4cb36fdadb99b2c2624253f86a0be3fa6cabbc09143e7d6d",
	// 2024-03-07 Releases
	"31be05170d7eb85493d09d248dd97fcdb0d3be89ca92bc0a7ef88b41959c317b736aaba0e79657ff4f6a1a35a7d8490f",
	"ed4a61117483ca11d0c89eb72423bfd0f86737d83eca522b95d6734bf455cf5a143d47a776c941dd801971a44d35ed63",
	// 2024-03-20 Release
	"a650498344fc8609d50a29a92d6868329eb47a810ee1143b1c7154349503163d14701ecdd9214ac9e71c1ef54448bf7b",
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
