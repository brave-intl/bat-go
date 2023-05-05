package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/mock"
	"os"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

var (
	mockBitflyerHost = "fake://bitflyer.com"
	// bitflyerBulkPayload = bitflyer.WithdrawToDepositIDBulkPayload{
	// 	DryRun:      true,
	// 	Withdrawals: []bitflyer.WithdrawToDepositIDPayload{},
	// 	PriceToken:  "",
	// 	DryRunOption: &bitflyer.DryRunOption{
	// 		RequestAPITransferStatus: "",
	// 		ProcessTimeSec:           1,
	// 		StatusAPITransferStatus:  "",
	// 	},
	// }
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

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

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
	bitflyerStateMachine := BitflyerMachine{}
	mockDriver := new(MockDriver)
	mockTxn := new(mockTransaction)
	mockRes := new(mockResult)
	mockDriver.On("Execute", context.Background(), mock.Anything).Return(mockRes, nil)
	mockTxn.On(
		"Execute",
		"SELECT * FROM history(transaction) AS h WHERE h.metadata.id = ?",
		mock.Anything,
	).Return(mockRes, nil)
	transaction := Transaction{State: Initialized}
	mockRes.On("Next", mockTxn).Return(true).Once()

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	newState, _ := Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Initialized, newState)

	// Create a sample state to represent the now-initialized entity.
	transaction.State = Prepared

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should transition transaction into the Authorized state
	newState, _ = Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)

	transaction.State = Authorized

	newState, _ = Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)

	transaction.State = Pending
	newState, _ = Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Paid, newState)

	transaction.State = Paid
	newState, _ = Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Paid, newState)

	transaction.State = Failed
	newState, _ = Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Failed, newState)
}

/*
TestBitflyerStateMachine500FailureToPaidTransition tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
*/
func TestBitflyerStateMachine500FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

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
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	mockDriver := new(MockDriver)
	transaction := Transaction{State: Prepared}
	bitflyerStateMachine := BitflyerMachine{}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 500

	newState, _ := Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)
}

/*
TestBitflyerStateMachine404FailureToPaidTransition tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
*/
func TestBitflyerStateMachine404FailureToPaidTransition(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("BITFLYER_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

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
	mockDriver := new(MockDriver)
	transaction := Transaction{State: Pending}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	bitflyerStateMachine := BitflyerMachine{}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 404

	newState, _ := Drive(ctx, &bitflyerStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)
}
