// +build integration

package payments

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"github.com/brave-intl/bat-go/libs/clients/zebpay"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/logging"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

/*
TestLiveZebpayStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
NOTICE: Because this test runs against the Zebpay staging environment, transaction details need to
be changed for each run. Prior transactions will exist in some state or other in Zebpay and that
breaks some tests.
*/
func TestLiveZebpayStateMachineHappyPathTransitions(t *testing.T) {
	ctx, _ := logging.SetupLogger(context.WithValue(context.Background(), appctx.DebugLoggingCTXKey, true))

	zebpayHost := "https://rewards.zebpay.co"
	err := os.Setenv("ZEBPAY_ENVIRONMENT", "test")
	must.Nil(t, err)
	err = os.Setenv("ZEBPAY_SERVER", zebpayHost)
	must.Nil(t, err)

	zebpayClient, err := zebpay.NewWithHTTPClient(http.Client{})
	must.Nil(t, err)
	signingKeyString := os.Getenv("ZEBPAY_TEST_SECRET")
	block, rest := pem.Decode([]byte(signingKeyString))
	if block == nil || block.Type != "PRIVATE KEY" || len(rest) != 0 {
		t.Log("failed pem decode")
	}
	signingKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Log("failed key parse")
	}

	zebpayStateMachine := ZebpayMachine{
		client:     zebpayClient,
		zebpayHost: zebpayHost,
		apiKey:     os.Getenv("ZEBPAY_TEST_API_KEY"),
		signingKey: signingKey,
	}

	idempotencyKey, err := uuid.Parse("1803df27-f29c-537a-9384-bb5b523ea3f7")
	must.Nil(t, err)

	testState := paymentLib.AuthenticatedPaymentState{
		Status: paymentLib.Prepared,
		PaymentDetails: paymentLib.PaymentDetails{
			Amount:    decimal.NewFromFloat(1.3),
			To:        "13460",
			From:      "c6911095-ba83-4aa1-b0fb-15934568a65a",
			Custodian: "zebpay",
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
	zebpayStateMachine.setTransaction(
		&testState,
	)

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should transition transaction into the Authorized state
	testState.Status = paymentLib.Prepared
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	newTransaction, err := Drive(ctx, &zebpayStateMachine)
	must.Nil(t, err)
	// Ensure that our Bitflyer calls are going through the mock and not anything else.
	//must.Equal(t, info[tokenInfoKey], 1)
	should.Equal(t, paymentLib.Authorized, newTransaction.Status)

	// Should transition transaction into the Pending state
	testState.Status = paymentLib.Authorized
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	timeout, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()
	// For this test, we could return Pending status forever, so we need it to time out
	// in order to capture and verify that pending status.
	newTransaction, err = Drive(timeout, &zebpayStateMachine)
	// The only tolerable error is a timeout, but if we get one here it's probably indicative of a
	// problem and we should look into it.
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Pending, newTransaction.Status)

	// Should transition transaction into the Paid state
	testState.Status = paymentLib.Pending
	marshaledData, _ = json.Marshal(testState)
	mockTransitionHistory.Data.UnsafePaymentState = marshaledData
	zebpayStateMachine.setTransaction(&testState)
	// This test shouldn't time out, but if it gets stuck in pending the defaul Drive timeout
	// is 5 minutes and we don't want the test to run that long even if it's broken.
	timeout, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	newTransaction, err = Drive(timeout, &zebpayStateMachine)
	must.Equal(t, nil, err)
	should.Equal(t, paymentLib.Paid, newTransaction.Status)
}
