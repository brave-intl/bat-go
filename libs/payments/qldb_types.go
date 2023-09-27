package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	qldbTypes "github.com/aws/aws-sdk-go-v2/service/qldb/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmsTypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// QLDBPaymentTransitionHistoryEntryBlockAddress defines blockAddress data for
// qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryBlockAddress struct {
	StrandID   string `ion:"strandId"`
	SequenceNo int64  `ion:"sequenceNo"`
}

// QLDBPaymentTransitionHistoryEntryHash defines hash for qldbPaymentTransitionHistoryEntry.
type QLDBPaymentTransitionHistoryEntryHash string

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
	for _, marshaledTransaction := range transactionHistory {
		verifyOutput, err := kmsClient.Verify(ctx, &kms.VerifyInput{
			KeyId:            &kmsSigningKeyID,
			Message:          marshaledTransaction.Data.UnsafePaymentState,
			Signature:        marshaledTransaction.Data.Signature,
			SigningAlgorithm: kmsTypes.SigningAlgorithmSpecEcdsaSha256,
		})
		if err != nil {
			return false, fmt.Errorf("failed to verify state signature: %e", err)
		}
		if !verifyOutput.SignatureValid {
			return false, fmt.Errorf("signature for state was not valid: %s", marshaledTransaction.Metadata.ID)
		}
	}
	return true, nil
}