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
	"math"
	"os"
	"testing"
	"time"

	"github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/rpc"
	"github.com/blocto/solana-go-sdk/types"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/google/uuid"
	"github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

/*
 * The below tests rely on some environment variables in order to execute:
 * SOLANA_SIGNING_KEY
 * SOLANA_RPC_ENDPOINT
 * SOLANA_PAYER_ADDRESS
 */

/*
TestLiveSolanaStateMachineATAMissing tests for correct state progression from
Initialized to Paid with a payee account that is missing the SPL-BAT ATA.
*/
func TestLiveSolanaStateMachineATAMissing(t *testing.T) {
	const (
		mint              = "AH86ZDiGbV1GSzqtJ6sgfUbXSXrGKKjju4Bs1Gm75AQq"
		tokenAccountOwner = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	)

	ctx, _ := logging.SetupLogger(context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, true))

	// New account for every test execution to ensure that the account does
	// not already have its ATA configured.
	payeeAccount := types.NewAccount()

	state := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(1.4),
			To:        payeeAccount.PublicKey.ToBase58(),
			From:      os.Getenv("SOLANA_PAYER_ADDRESS"),
			Custodian: "solana",
			PayoutID:  "4b2f22c9-f227-43b3-98d2-4a5337b65bc5",
			Currency:  "BAT",
		},
		Authorizations: []paymentLib.PaymentAuthorization{{}, {}, {}},
	}

	solMachine, mockTransitionHistory, marshaledState := setupState(state, mint, t)

	finalState := driveHappyPathTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solMachine,
		marshaledState,
		t,
	)

	createdAta, _, err := common.FindAssociatedTokenAddress(
		payeeAccount.PublicKey,
		common.PublicKeyFromString(mint),
	)
	must.Nil(t, err)
	solClient := client.NewClient(solMachine.solanaRpcEndpoint)
	must.Nil(t, err)
	// The RPC server caches the result of GetAccountInfo and the new value is not returned
	// for over 10 seconds in our testing. Therefore, ugly as it is, loop until we get a result
	// that matches our expectations or we give up and consider it a failure.
	var result client.AccountInfo
	for i := 1; i < 30; i++ {
		t.Log("Checking that ATA was created")
		time.Sleep(1 * time.Second)
		result, err = solClient.GetAccountInfo(ctx, createdAta.ToBase58())
		must.Nil(t, err)
		if tokenAccountOwner == result.Owner.ToBase58() {
			break
		}
	}
	// Check if the ATA was created by checking it's "Owner", which is shared by all SPL tokens
	// regardless of who created them. This just determines whether the ATA exists by checking that
	// it has an Owner field that is valid.
	should.Equal(t, tokenAccountOwner, result.Owner.ToBase58())

	checkTransactionMatchesPaymentDetails(ctx, t, solMachine.solanaRpcEndpoint, createdAta, state, *finalState)
}

/*
TestLiveSolanaStateMachineATAPresent tests for correct state progression from
Initialized to Paid with a payee account that has the SPL-BAT ATA configured.
*/
func TestLiveSolanaStateMachineATAPresent(t *testing.T) {
	const (
		mint = "AH86ZDiGbV1GSzqtJ6sgfUbXSXrGKKjju4Bs1Gm75AQq"
	)

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

	solMachine, mockTransitionHistory, marshaledState := setupState(state, mint, t)

	finalState := driveHappyPathTransitions(
		ctx,
		state,
		mockTransitionHistory,
		solMachine,
		marshaledState,
		t,
	)

	staticAta, _, err := common.FindAssociatedTokenAddress(
		common.PublicKeyFromString(state.PaymentDetails.To),
		common.PublicKeyFromString(mint),
	)
	must.Nil(t, err)

	checkTransactionMatchesPaymentDetails(ctx, t, solMachine.solanaRpcEndpoint, staticAta, state, *finalState)
}

func checkTransactionMatchesPaymentDetails(
	ctx context.Context,
	t *testing.T,
	endpoint string,
	ata common.PublicKey,
	state, finalState paymentLib.AuthenticatedPaymentState,
) {
	solanaData := chainIdempotencyData{}
	err := json.Unmarshal(finalState.ExternalIdempotency, &solanaData)
	must.Nil(t, err)

	solClient := client.NewClient(endpoint)
	must.Nil(t, err)

	var txn rpc.JsonRpcResponse[*rpc.GetTransaction]
	t.Log("Fetching transaction data")
	b58Signature := base58.Encode(solanaData.Transaction.Signatures[0])
	txn, err = solClient.RpcClient.GetTransactionWithConfig(ctx, b58Signature, rpc.GetTransactionConfig{
		Encoding:   rpc.TransactionEncodingJsonParsed,
		Commitment: rpc.CommitmentConfirmed,
	})
	must.Nil(t, err)
	if innerTxn, ok := txn.Result.Transaction.(map[string]interface{}); ok {
		if message, ok := innerTxn["message"].(map[string]interface{}); ok {
			if instructions, ok := message["instructions"].([]interface{}); ok {
				if instructionOne, ok := instructions[0].(map[string]interface{}); ok {
					if parsed, ok := instructionOne["parsed"].(map[string]interface{}); ok {
						if info, ok := parsed["info"].(map[string]interface{}); ok {
							t.Log("Verifying chain transaction mint, to, and from")
							should.Equal(t, "AH86ZDiGbV1GSzqtJ6sgfUbXSXrGKKjju4Bs1Gm75AQq", info["mint"])
							should.Equal(t, state.PaymentDetails.To, info["wallet"])
							should.Equal(t, state.PaymentDetails.From, info["source"])
						} else {
							t.Fail()
						}
					} else {
						t.Fail()
					}
				} else {
					t.Fail()
				}
				if instructionTwo, ok := instructions[1].(map[string]interface{}); ok {
					if parsed, ok := instructionTwo["parsed"].(map[string]interface{}); ok {
						if info, ok := parsed["info"].(map[string]interface{}); ok {
							t.Log("Verifying chain transaction amount, ATA, and from")
							amount := state.PaymentDetails.Amount.Mul(
								decimal.NewFromFloat(math.Pow10(int(8))),
							).BigInt().Uint64()
							should.Equal(t, fmt.Sprint(amount), info["amount"])
							should.Equal(t, ata.ToBase58(), info["destination"])
							should.Equal(t, state.PaymentDetails.From, info["authority"])
						} else {
							t.Fail()
						}
					} else {
						t.Fail()
					}
				} else {
					t.Fail()
				}
			} else {
				t.Fail()
			}
		} else {
			t.Fail()
		}
	} else {
		t.Fail()
	}
}

func driveHappyPathTransitions(
	ctx context.Context,
	testState paymentLib.AuthenticatedPaymentState,
	mockTransitionHistory QLDBPaymentTransitionHistoryEntry,
	solMachine SolanaMachine,
	marshaledData []byte,
	t *testing.T,
) *paymentLib.AuthenticatedPaymentState {
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
	must.NotNil(t, transaction.ExternalIdempotency)
	persistedIdempotency := chainIdempotencyData{}
	err := json.Unmarshal(transaction.ExternalIdempotency, &persistedIdempotency)
	must.Nil(t, err)
	must.NotNil(t, persistedIdempotency.Transaction)
	must.NotNil(t, persistedIdempotency.BlockHash)
	must.NotNil(t, persistedIdempotency.SlotTarget)
	should.Equal(t, 1, len(persistedIdempotency.Transaction.Signatures))
	t.Log("State is Authorized")

	// Should transition transaction into the Pending state
	// For this test, we could return Pending status forever, so we need it to time out
	// in order to capture and verify that pending status.
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	transaction = transitioner(timeout, *transaction, paymentLib.Authorized, paymentLib.Pending)
	should.Equal(t, paymentLib.Pending, transaction.Status)
	t.Log("State is Pending")
	idempotencyData := transaction.ExternalIdempotency

	// Should transition transaction into the Paid state
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	transaction = transitioner(timeout, *transaction, paymentLib.Pending, paymentLib.Paid)
	should.Equal(t, paymentLib.Pending, transaction.Status)
	should.Equal(t, idempotencyData, transaction.ExternalIdempotency)
	t.Log("State is Pending")

	for i := 1; i < 30; i++ {
		t.Log("Checking for Paid status")
		time.Sleep(100 * time.Millisecond)
		md, _ := json.Marshal(transaction)
		mockTransitionHistory.Data.UnsafePaymentState = md
		solMachine.setTransaction(transaction)
		transaction, _ = Drive(ctx, &solMachine)
		should.Equal(t, idempotencyData, transaction.ExternalIdempotency)
		if transaction.Status == paymentLib.Paid {
			break
		}
	}
	should.Equal(t, paymentLib.Paid, transaction.Status)
	should.Equal(t, idempotencyData, transaction.ExternalIdempotency)
	t.Log("State is Paid")

	return transaction
}

func setupState(
	state paymentLib.AuthenticatedPaymentState,
	mint string,
	t *testing.T,
) (
	SolanaMachine,
	QLDBPaymentTransitionHistoryEntry,
	[]byte,
) {
	solMachine := SolanaMachine{
		signingKey:        os.Getenv("SOLANA_SIGNING_KEY"),
		solanaRpcEndpoint: os.Getenv("SOLANA_RPC_ENDPOINT"),
		splMintAddress:    mint, // SPL mint address on devnet
		splMintDecimals:   8,    // SPL mint decimals on devnet
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
	solMachine.setTransaction(&state)
	return solMachine, mockTransitionHistory, marshaledData
}

func getTransitioner(
	ctx context.Context,
	mth QLDBPaymentTransitionHistoryEntry,
	sm SolanaMachine,
	t *testing.T,
) func(
	ctx context.Context,
	state paymentLib.AuthenticatedPaymentState,
	start, end paymentLib.PaymentStatus,
) *paymentLib.AuthenticatedPaymentState {
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
