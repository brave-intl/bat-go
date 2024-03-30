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
	"github.com/blocto/solana-go-sdk/types"
)

const (
	splMintAddress  string = "AH86ZDiGbV1GSzqtJ6sgfUbXSXrGKKjju4Bs1Gm75AQq" // SPL mint address on devnet
	splMintDecimals uint8  = 8                                              // SPL mint decimals on devnet
)

/*
TestLiveSolanaStateMachineATAMissing tests for correct state progression from
Initialized to Paid with a payee account that is missing the SPL-BAT ATA.
*/
func TestLiveSolanaStateMachine(t *testing.T) {
	ctx, _ := logging.SetupLogger(context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, true))

	// New account for every test execution to ensure that the account does
	// not already have its ATA configured.
	payee_account := types.NewAccount()

	state := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(1.4),
			To:        string(payee_account.PublicKey[:]),
			From:      os.Getenv("SOLANA_PAYER_ADDRESS"),
			Custodian: "solana",
			PayoutID:  "4b2f22c9-f227-43b3-98d2-4a5337b65bc5",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	solanaStateMachine, mockTransitionHistory, marshaledState := setupState(state, t)

	driveTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solanaStateMachine,
		marshaledState,
		t,
	)
}

/*
TestLiveSolanaStateMachineATAPresent tests for correct state progression from
Initialized to Paid with a payee account that has the SPL-BAT ATA configured.
*/
func TestLiveSolanaStateMachineATAPresent(t *testing.T) {
	ctx, _ := logging.SetupLogger(context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, true))

	state := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(1.4),
			// Fixed To address that has the ATA configured already
			To:        "5g7xMFn9bk8vyZdfsr4mAfUWKWDaWxzZBH5Cb1XHftBL",
			From:      os.Getenv("SOLANA_PAYER_ADDRESS"),
			Custodian: "solana",
			PayoutID:  "4b2f22c9-f227-43b3-98d2-4a5337b65bc5",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	solanaStateMachine, mockTransitionHistory, marshaledState := setupState(state, t)

	driveTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solanaStateMachine,
		marshaledState,
		t,
	)
}

func setupState(
	state paymentLib.AuthenticatedPaymentState,
	t *testing.T,
) (
	SolanaMachine,
	QLDBPaymentTransitionHistoryEntry,
	[]byte,
) {
	sm := SolanaMachine{
		signingKey:        os.Getenv("SOLANA_SIGNING_KEY"),
		solanaRpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
		splMintAddress:    splMintAddress,
		splMintDecimals:   splMintDecimals,
	}
	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
	must.Nil(t, err)

	marshaledData, _ := json.Marshal(state)
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
	solMachine.setTransaction(
		&state,
	)
	return sm, mockTransitionHistory, marshaledData
}

func driveTransitions(
	ctx context.Context,
	testState paymentLib.AuthenticatedPaymentState,
	mockTransitionHistory QLDBPaymentTransitionHistoryEntry,
	solanaStateMachine SolanaMachine,
	marshaledData []byte,
	t *testing.T,
) {
	// Should transition transaction into the Authorized state
	testState.Status = paymentLib.Prepared
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solMachine.setTransaction(&state)
	authTxn, err := Drive(ctx, &solMachine)
	must.Nil(t, err)
	should.Equal(t, paymentLib.Authorized, authTxn.Status)

	// State transition: Authorized -> Pending
	marshaledData, _ = json.Marshal(authTxn)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solMachine.setTransaction(authTxn)
	// Set a timeout long enough to allow for the transaction to be submitted
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pendingTransaction, err := Drive(timeout, &solMachine)
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Pending, pendingTransaction.Status)

	// State transition: Pending -> Paid
	marshaledData, _ = json.Marshal(pendingTransaction)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	solanaStateMachine.setTransaction(&testState)
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	timeout, cancel = context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
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
