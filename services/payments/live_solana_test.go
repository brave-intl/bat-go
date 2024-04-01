// +build integrationsolana

package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
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
			To:        os.Getenv("SOLANA_PAYEE_ADDRESS_WITH_ATA"),
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

	// State transition: Prepared -> Authorized
	testState.Status = paymentLib.Prepared
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(&testState)
	authorizedTransaction, err := Drive(ctx, &solanaStateMachine)
	must.Nil(t, err)
	should.Equal(t, paymentLib.Authorized, authorizedTransaction.Status)

	// State transition: Authorized -> Pending
	marshaledData, _ = json.Marshal(authorizedTransaction)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(authorizedTransaction)
	// Set a timeout long enough to allow for the transaction to be submitted
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pendingTransaction, err := Drive(timeout, &solanaStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Pending, pendingTransaction.Status)

	// State transition: Pending -> Paid
	marshaledData, _ = json.Marshal(pendingTransaction)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(pendingTransaction)
	// Set a timeout equivalent to 150 slots, long enough to allow for the transaction to be confirmed
	timeout, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	var (
		paidTransaction *paymentLib.AuthenticatedPaymentState
	)
	for {
		paidTransaction, err = Drive(timeout, &solanaStateMachine)
		must.Equal(t, nil, err)
		if paidTransaction.Status == paymentLib.Paid {
			break
		}
	}

	should.Equal(t, paymentLib.Paid, paidTransaction.Status)
}
