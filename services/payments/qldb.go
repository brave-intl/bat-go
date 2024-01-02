package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/aws/aws-sdk-go-v2/service/qldb"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	appctx "github.com/brave-intl/bat-go/libs/context"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
)

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash []byte

/* TODO: unused
// qldbPaymentTransitionHistoryEntrySignature defines signature for qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte
*/

// QLDBPaymentTransitionHistoryEntryMetadata defines metadata for qldbPaymentTransitionHistoryEntry
type QLDBPaymentTransitionHistoryEntryMetadata struct {
	ID      string    `ion:"id"`
	TxID    string    `ion:"txId"`
	TxTime  time.Time `ion:"txTime"`
	Version int64     `ion:"version"`
}

// QLDBPaymentTransitionHistoryEntry defines top level entry for a QLDB transaction.
type QLDBPaymentTransitionHistoryEntry struct {
	BlockAddress QLDBPaymentTransitionHistoryEntryBlockAddress `ion:"blockAddress"`
	Hash         QLDBPaymentTransitionHistoryEntryHash         `ion:"hash"`
	Data         paymentLib.PaymentState                       `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder.
func (b *QLDBPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

// revisionValidInTree verifies a document revision in QLDB using a digest and the Merkle
// hashes to re-derive the digest.
func revisionValidInTree(
	ctx context.Context,
	client wrappedQldbSDKClient,
	transaction *QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	qldbLedgerName, ok := ctx.Value(appctx.PaymentsQLDBLedgerNameCTXKey).(string)
	if !ok {
		return false, fmt.Errorf("empty qldb ledger name. revision not verified for state: %v", transaction)
	}
	digest, err := client.GetDigest(ctx, &qldb.GetDigestInput{Name: &qldbLedgerName})

	if err != nil {
		return false, fmt.Errorf("Failed to get digest: %w", err)
	}

	revision, err := client.GetRevision(ctx, &qldb.GetRevisionInput{
		BlockAddress:     transaction.BlockAddress.ValueHolder(),
		DocumentId:       &transaction.Metadata.ID,
		Name:             &qldbLedgerName,
		DigestTipAddress: digest.DigestTipAddress,
	})

	if err != nil {
		return false, fmt.Errorf("Failed to get revision: %w", err)
	}
	var hashes [][32]byte

	// This Ion unmarshal gives us the hashes as bytes. The documentation implies that
	// these are base64 encoded strings, but testing indicates that is not the case.
	err = ion.UnmarshalString(*revision.Proof.IonText, &hashes)

	if err != nil {
		return false, fmt.Errorf("Failed to unmarshal revision proof: %w", err)
	}
	return verifyHashSequence(digest, transaction.Hash, hashes)
}

func verifyHashSequence(
	digest *qldb.GetDigestOutput,
	initialHash QLDBPaymentTransitionHistoryEntryHash,
	hashes [][32]byte,
) (bool, error) {
	var concatenatedHash [32]byte
	for i, providedHash := range hashes {
		// During the first integration concatenatedHash hasn't been populated.
		// Populate it with the hash from the provided transaction.
		if i == 0 {
			decodedHash, err := base64.StdEncoding.DecodeString(string(initialHash))
			if err != nil {
				return false, err
			}
			copy(concatenatedHash[:], decodedHash)
		}
		// QLDB determines hash order by comparing the hashes byte by byte until
		// one is greater than the other. The larger becomes the left hash and the
		// smaller becomes the right hash for the next phase of hash generation.
		// This is not documented, but can be inferred from the Java reference
		// implementation here: https://github.com/aws-samples/amazon-qldb-dmv-sample-java/blob/master/src/main/java/software/amazon/qldb/tutorial/Verifier.java#L60
		sortedHashes, err := sortHashes(providedHash[:], concatenatedHash[:])
		if err != nil {
			return false, err
		}
		// Concatenate the hashes and then hash the result to get the next hash
		// in the tree.
		concatenatedHash = sha256.Sum256(append(sortedHashes[0], sortedHashes[1]...))
	}

	// The digest comes to us as a base64 encoded string. We need to decode it before
	// using it.
	decodedDigest, err := base64.StdEncoding.DecodeString(string(digest.Digest))

	if err != nil {
		return false, fmt.Errorf("Failed to base64 decode digest: %w", err)
	}

	if bytes.Compare(concatenatedHash[:], decodedDigest) == 0 {
		return true, nil
	}
	return false, nil
}

func sortHashes(a, b []byte) ([][]byte, error) {
	if len(a) != len(b) {
		return nil, errors.New("provided hashes do not have matching length")
	}
	for i := 0; i < len(a); i++ {
		if a[i] > b[i] {
			return [][]byte{a, b}, nil
		} else if a[i] < b[i] {
			return [][]byte{b, a}, nil
		}
	}
	return [][]byte{a, b}, nil
}
