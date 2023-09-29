package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"

	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
)

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash []byte

// qldbPaymentTransitionHistoryEntrySignature defines signature for
// qldbPaymentTransitionHistoryEntry.
type qldbPaymentTransitionHistoryEntrySignature []byte

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
	Data         PaymentState                                  `ion:"data"`
	Metadata     QLDBPaymentTransitionHistoryEntryMetadata     `ion:"metadata"`
}

// ValueHolder converts a QLDBPaymentTransitionHistoryEntry into a QLDB SDK ValueHolder.
func (b *QLDBPaymentTransitionHistoryEntryBlockAddress) ValueHolder() *qldbTypes.ValueHolder {
	stringValue := fmt.Sprintf("{strandId:\"%s\",sequenceNo:%d}", b.StrandID, b.SequenceNo)
	return &qldbTypes.ValueHolder{
		IonText: &stringValue,
	}
}

func (e *QLDBPaymentTransitionHistoryEntry) toTransaction() (*AuthenticatedPaymentState, error) {
	var txn AuthenticatedPaymentState
	err := json.Unmarshal(e.Data.UnsafePaymentState, &txn)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to unmarshal record data for conversion from qldbPaymentTransitionHistoryEntry to Transaction: %w",
			err,
		)
	}
	return &txn, nil
}

func AuthenticatedStateFromQLDBHistory(
	ctx context.Context,
	kmsClient wrappedKMSClient,
	kmsSigningKeyID string,
	stateHistory []QLDBPaymentTransitionHistoryEntry,
	paymentState PaymentState,
) (*AuthenticatedPaymentState, *QLDBPaymentTransitionHistoryEntry, error) {
	latestHistoryEntry, err := validatePaymentStateHistory(ctx, stateHistory)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate history: %w", err)
	}
	signaturesAreValid, err := validatePaymentStateSignatures(
		ctx,
		kmsClient,
		kmsSigningKeyID,
		stateHistory,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate signatures: %w", err)
	}
	if latestHistoryEntry == nil || !signaturesAreValid {
		return nil, nil, fmt.Errorf("state history failed validation: %v", paymentState)
	}
	authenticatedPaymentState, err := paymentState.ToStructuredUnsafePaymentState()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get authenticated state from payment state: %w", err)
	}
	return authenticatedPaymentState, latestHistoryEntry, nil
}

// validatePaymentStateHistory returns whether a slice of entries representing the entire state
// history for a given id include exclusively valid state transitions.
func validatePaymentStateHistory(
	ctx context.Context,
	transactionHistory []QLDBPaymentTransitionHistoryEntry,
) (*QLDBPaymentTransitionHistoryEntry, error) {
	var (
		reason                    error
		err                       error
		unmarshaledTransactionSet []AuthenticatedPaymentState
	)
	// Unmarshal the transactions in advance so that we don't have to do it multiple
	// times per transaction in the next loop.
	for _, marshaledTransaction := range transactionHistory {
		var transaction AuthenticatedPaymentState
		err = json.Unmarshal(marshaledTransaction.Data.UnsafePaymentState, &transaction)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction data: %w", err)
		}
		unmarshaledTransactionSet = append(unmarshaledTransactionSet, transaction)
	}
	for i, transaction := range unmarshaledTransactionSet {
		// Transitions must always start at 0
		if i == 0 {
			if transaction.Status != Prepared {
				return nil, &InvalidTransitionState{}
			}
			continue
		}

		// Now that the data itself is verified, proceed to check transition States.
		previousTransitionData := unmarshaledTransactionSet[i-1]
		// New transaction state should be present in the list of valid next states for the
		// "previous" (current) state.
		if !previousTransitionData.NextStateValid(transaction.Status) {
			return nil, &InvalidTransitionState{
				From: string(previousTransitionData.Status),
				To:   string(transaction.Status),
			}
		}
	}
	return &transactionHistory[0], reason
}

// validatePaymentStateSignatures returns whether a slice of entries representing the entire state
// history for a given id include exclusively valid signatures.
func validatePaymentStateSignatures(
	ctx context.Context,
	kmsClient wrappedKMSClient,
	kmsSigningKeyID string,
	transactionHistory []QLDBPaymentTransitionHistoryEntry,
) (bool, error) {
	for _, historyEntry := range transactionHistory {
		verifyOutput, err := kmsClient.Verify(ctx, &kms.VerifyInput{
			KeyId:            &kmsSigningKeyID,
			Message:          historyEntry.Data.UnsafePaymentState,
			MessageType:      kmsTypes.MessageTypeRaw,
			Signature:        historyEntry.Data.Signature,
			SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
		})
		if err != nil {
			return false, fmt.Errorf("failed to verify state signature: %e", err)
		}
		// If signature verification fails with the current enclave, check if the signature is valid
		// for the key that is persisted on the record itself. Only do this check for if the public
		// key is in the list of valid prior keys.
		// Note: KMS is hashing the value internally during signing, so we need to verify a hash of
		// the message as well when we're verifying without KMS.
		if !verifyOutput.SignatureValid {
			isValidPriorKey, err := publicKeyInHistoricalAuthorizedKeySet(
				historyEntry.Data.PublicKey,
			)
			if err != nil || !isValidPriorKey {
				return false, fmt.Errorf(
					"key could not be found in list of valid prior keys: %w",
					err,
				)
			}
			pubkeyParsed, err := x509.ParsePKIXPublicKey(historyEntry.Data.PublicKey)
			if err != nil {
				return false, fmt.Errorf(
					"failed to unmarshal public key for prior key comparison: %w",
					err,
				)
			}
			pubkey, ok := pubkeyParsed.(*ecdsa.PublicKey)
			if !ok {
				return false, fmt.Errorf(
					"public key was of the wrong type for document ID %s",
					historyEntry.Metadata.ID,
				)
			}

			hash := sha256.New()
			hash.Write(historyEntry.Data.UnsafePaymentState)

			pubkeyVerified := ecdsa.VerifyASN1(
				pubkey,
				hash.Sum(nil),
				historyEntry.Data.Signature,
			)

			if !pubkeyVerified {
				return false, fmt.Errorf(
					"signature for state with document ID %s was not valid",
					historyEntry.Metadata.ID,
				)
			}
		}
	}
	return true, nil
}

// publicKeyInHistoricalAuthorizedKeySet checks if the hex encoded, marshalled representation of the
// provided public key is present in a list of valid prior public keys.
func publicKeyInHistoricalAuthorizedKeySet(pubkey []byte) (bool, error) {
	priorPubkeys := []string{}
	base64Pubkey := base64.StdEncoding.EncodeToString(pubkey)
	if !slices.Contains(priorPubkeys, base64Pubkey) {
		return false, errors.New("provided public key is not in the list of valid prior keys")
	}
	return true, nil
}
