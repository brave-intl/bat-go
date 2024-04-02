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

	"github.com/blocto/solana-go-sdk/types"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
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

	solMachine, mockTransitionHistory, marshaledState := setupState(state, t)

	driveHappyPathTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solMachine,
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
			Amount: decimal.NewFromFloat(1.4),
			// Fixed To address that has the ATA configured already
			To:        "5g7xMFn9bk8vyZdfsr4mAfUWKWDaWxzZBH5Cb1XHftBL",
			From:      os.Getenv("SOLANA_PAYER_ADDRESS"),
			Custodian: "solana",
			PayoutID:  "4b2f22c9-f227-43b3-98d2-4a5337b65bc5",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	solMachine, mockTransitionHistory, marshaledState := setupState(state, t)

	driveHappyPathTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solMachine,
		marshaledState,
		t,
	)
}

func driveHappyPathTransitions(
	ctx context.Context,
	testState paymentLib.AuthenticatedPaymentState,
	mockTransitionHistory QLDBPaymentTransitionHistoryEntry,
	solMachine SolanaMachine,
	marshaledData []byte,
	t *testing.T,
) {
	var transaction *paymentLib.AuthenticatedPaymentState
	transitioner := getTransitioner(
		ctx,
		mockTransitionHistory,
		solMachine,
		t,
	)

	// Should transition transaction into the Authorized state
	transaction = transitioner(ctx, testState, paymentLib.Prepared, paymentLib.Authorized)
	should.Equal(t, paymentLib.Authorized, transaction.Status)
	fmt.Printf("STATUS 1: %s\n", transaction.ExternalIdempotency)

	// Should transition transaction into the Pending state
	// For this test, we could return Pending status forever, so we need it to time out
	// in order to capture and verify that pending status.
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	transaction = transitioner(timeout, *transaction, paymentLib.Authorized, paymentLib.Pending)
	should.Equal(t, paymentLib.Pending, transaction.Status)
	fmt.Printf("STATUS 2: %s\n", transaction.ExternalIdempotency)

	// Should transition transaction into the Paid state
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	transaction = transitioner(timeout, *transaction, paymentLib.Pending, paymentLib.Paid)
	should.Equal(t, paymentLib.Pending, transaction.Status)
	fmt.Printf("STATUS 3: %s\n", transaction.ExternalIdempotency)

	if transaction.Status != paymentLib.Paid {
		for i := 1; i < 3; i++ {
			time.Sleep(5 * time.Second)
			md, _ := json.Marshal(transaction)
			mockTransitionHistory.Data.UnsafePaymentState = md
			solMachine.setTransaction(transaction)
			transaction, _ = Drive(ctx, &solMachine)
			fmt.Printf("STATUS 4: %s\n", transaction.ExternalIdempotency)
			if transaction.Status == paymentLib.Paid {
				break
			}
		}
	}
	should.Equal(t, paymentLib.Paid, transaction.Status)
}

func setupState(
	state paymentLib.AuthenticatedPaymentState,
	t *testing.T,
) (
	SolanaMachine,
	QLDBPaymentTransitionHistoryEntry,
	[]byte,
) {
	const (
		splMintAddress  string = "AH86ZDiGbV1GSzqtJ6sgfUbXSXrGKKjju4Bs1Gm75AQq" // SPL mint address on devnet
		splMintDecimals uint8  = 8                                              // SPL mint decimals on devnet
	)

	solMachine := SolanaMachine{
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
	return solMachine, mockTransitionHistory, marshaledData
}

func getTransitioner(
	ctx context.Context,
	mth QLDBPaymentTransitionHistoryEntry,
	sm SolanaMachine,
	t *testing.T,
) func(ctx context.Context, state paymentLib.AuthenticatedPaymentState, start, end paymentLib.PaymentStatus) *paymentLib.AuthenticatedPaymentState {
	return func(
		ctx context.Context,
		state paymentLib.AuthenticatedPaymentState,
		start, end paymentLib.PaymentStatus,
	) *paymentLib.AuthenticatedPaymentState {
		return transition(ctx, state, mth, sm, start, end, t)
	}
}

func transition(
	ctx context.Context,
	ts paymentLib.AuthenticatedPaymentState,
	mth QLDBPaymentTransitionHistoryEntry,
	sm SolanaMachine,
	start paymentLib.PaymentStatus,
	end paymentLib.PaymentStatus,
	t *testing.T,
) *paymentLib.AuthenticatedPaymentState {
	ts.Status = start
	md, _ := json.Marshal(ts)
	mth.Data.UnsafePaymentState = md
	sm.setTransaction(&ts)
	newTransaction, err := Drive(ctx, &sm)
	must.Nil(t, err)
	return newTransaction
}
