package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

var (
	mockBitflyerHost    = "fake://bitflyer.com"
	bitflyerBulkPayload = bitflyer.WithdrawToDepositIDBulkPayload{
		DryRun:      true,
		Withdrawals: []bitflyer.WithdrawToDepositIDPayload{},
		PriceToken:  "",
		DryRunOption: &bitflyer.DryRunOption{
			RequestAPITransferStatus: "",
			ProcessTimeSec:           1,
			StatusAPITransferStatus:  "",
		},
	}
)

type ctxAuthKey struct{}

/*
TestBitflyerStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestBitflyerStateMachineHappyPathTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	os.Setenv("BITFLYER_ENVIRONMENT", "test")

	// Mock transaction creation that will succeed
	jsonResponse, err := json.Marshal(bitflyerTransactionSubmitSuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-request",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)
	// Mock transaction commit that will succeed
	jsonResponse, err = json.Marshal(bitflyerTransactionCheckStatusSuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	ctx := context.Background()

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	newState, _ := DriveBitflyerTransaction(ctx, QLDBPaymentTransitionHistoryEntry{}, bitflyerBulkPayload)
	assert.Equal(t, Initialized, newState)

	// Create a sample state to represent the now-initialized entity.
	currentState := QLDBPaymentTransitionHistoryEntry{}

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	currentState.Data.Status = 1
	currentState.Metadata.Version = 1

	currentState.Data.Status = 2
	newState, _ = DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Pending, newState)

	currentState.Data.Status = 3
	newState, _ = DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Paid, newState)

	currentState.Data.Status = 4
	newState, _ = DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Paid, newState)

	currentState.Data.Status = 5
	newState, _ = DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Failed, newState)
}

/*
TestBitflyerStateMachine500FailureToPaidTransition tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
*/
func TestBitflyerStateMachine500FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	os.Setenv("BITFLYER_ENVIRONMENT", "test")

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(bitflyerTransactionSubmitFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	currentState := QLDBPaymentTransitionHistoryEntry{}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	currentState.Data.Status = 2
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	currentState.Metadata.Version = 500

	newState, _ := DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Authorized, newState)
}

/* TestBitflyerStateMachine404FailureToPaidTransition tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
*/
func TestBitflyerStateMachine404FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	os.Setenv("BITFLYER_ENVIRONMENT", "test")

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(bitflyerTransactionCheckStatusFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/api/link/v1/coin/withdraw-to-deposit-id/bulk-status",
			mockBitflyerHost,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	currentState := QLDBPaymentTransitionHistoryEntry{}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	currentState.Data.Status = 3
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	currentState.Metadata.Version = 404

	newState, _ := DriveBitflyerTransaction(ctx, currentState, bitflyerBulkPayload)
	assert.Equal(t, Pending, newState)
}
