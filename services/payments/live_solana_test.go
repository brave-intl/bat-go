//go:build integrationsolana
// +build integrationsolana

package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

/*
TestLiveSolanaStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid with a payee account that is missing the SPL-BAT ATA.
*/
func TestLiveSolanaStateMachineATAMissing(t *testing.T) {
	ctx, _ := logging.SetupLogger(context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, true))

	solanaStateMachine := SolanaMachine{
		signingKey:        os.Getenv("SOLANA_SIGNING_KEY"),
		solanaRpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
	}

	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
	must.Nil(t, err)

	testState := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(1.4),
			To:        os.Getenv("SOLANA_PAYEE_ADDRESS_WITHOUT_ATA"),
			From:      os.Getenv("SOLANA_PAYER_ADDRESS"),
			Custodian: "solana",
			PayoutID:  "4b2f22c9-f227-43b3-98d2-4a5337b65bc5",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	marshaledData, _ := json.Marshal(testState)
	must.Nil(t, err)
	privkey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must.Nil(t, err)
	marshalledPubkey, err := x509.MarshalPKIXPublicKey(&privkey.PublicKey)
	must.Nil(t, err)
	mockTransitionHistory := QLDBPaymentTransitionHistoryEntry{
		BlockAddress: QLDBPaymentTransitionHistoryEntryBlockAddress{
			StrandID:   "test",
			SequenceNo: 1,
		},
		Hash: []byte("test"),
		Data: paymentLib.PaymentState{
			UnsafePaymentState: marshaledData,
			Signature:          []byte{},
			ID:                 idempotencyKey,
			PublicKey:          string(marshalledPubkey),
		},
		Metadata: QLDBPaymentTransitionHistoryEntryMetadata{
			ID:      "test",
			Version: 1,
			TxTime:  time.Now(),
			TxID:    "test",
		},
	}
	solanaStateMachine.setTransaction(
		&testState,
	)

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should transition transaction into the Authorized state
	testState.Status = paymentLib.Prepared
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(&testState)
	newTransaction, err := Drive(ctx, &solanaStateMachine)
	must.Nil(t, err)
	should.Equal(t, paymentLib.Authorized, newTransaction.Status)

	// Should transition transaction into the Pending state
	testState.Status = paymentLib.Authorized
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(&testState)
	timeout, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	// For this test, we could return Pending status forever, so we need it to time out
	// in order to capture and verify that pending status.
	newTransaction, err = Drive(timeout, &solanaStateMachine)
	// The only tolerable error is a timeout, but if we get one here it's probably indicative of a
	// problem and we should look into it.
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Pending, newTransaction.Status)

	// Should transition transaction into the Paid state
	testState.Status = paymentLib.Pending
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(&testState)
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	timeout, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	// TODO Handle the missing from chain case where the transaction was sent but can't
	// yet be found. Until that's done, wait here a moment.
	//time.Sleep(5*time.Second)
	newTransaction, err = Drive(timeout, &solanaStateMachine)
	fmt.Printf("STATUS: %s\n", newTransaction.Status)
	must.Equal(t, nil, err)
	for i := 1; i < 3; i++ {
		time.Sleep(1 * time.Second)
		newTransaction, err = Drive(timeout, &solanaStateMachine)
		fmt.Printf("STATUS: %s\n", newTransaction.Status)
		if newTransaction.Status == paymentLib.Paid {
			break
		}
	}
	should.Equal(t, paymentLib.Paid, newTransaction.Status)
}
