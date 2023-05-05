package payments

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

var (
	mockUpholdHost = "fake://mock.uphold.com"

	upholdWallet = uphold.Wallet{
		Info: walletutils.Info{
			ProviderID: "test",
			Provider:   "uphold",
			PublicKey:  "",
		},
		PrivKey: ed25519.PrivateKey([]byte("")),
		PubKey:  httpsignature.Ed25519PubKey([]byte("")),
	}
	upholdSucceedTransaction = custodian.Transaction{ProviderID: "1234"}
)

/*
TestUpholdStateMachineHappyPathTransitions tests for correct state progression from
Initialized to Paid. Additionally, Paid status should be final and Failed status should
be permanent.
*/
func TestUpholdStateMachineHappyPathTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("UPHOLD_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

	// Mock transaction creation
	jsonResponse, err := json.Marshal(upholdCreateTransactionSuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)
	// Mock transaction commit that will succeed
	jsonResponse, err = json.Marshal(upholdCommitTransactionSuccessResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions/%s/commit",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
			upholdSucceedTransaction.ProviderID,
		),
		httpmock.NewStringResponder(200, string(jsonResponse)),
	)

	ctx := context.Background()
	upholdStateMachine := UpholdMachine{}
	mockDriver := new(mockDriver)
	transaction := Transaction{State: Initialized}

	// Should create a transaction in QLDB. Current state argument is empty because
	// the object does not yet exist.
	newState, _ := Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Initialized, newState)

	// Create a sample state to represent the now-initialized entity.
	transaction.State = Prepared

	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")

	// Should transition transaction into the Authorized state
	newState, _ = Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)

	transaction.State = Authorized
	// Should transition transaction into the Pending state
	newState, _ = Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)

	transaction.State = Pending
	// Should transition transaction into the Paid state
	newState, _ = Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Paid, newState)
}

/*
TestUpholdStateMachine500FailureToPendingTransitions tests for a failure to progress status
after a 500 error response while attempting to transfer from Pending to Paid
*/
func TestUpholdStateMachine500FailureToPendingTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	os.Setenv("UPHOLD_ENVIRONMENT", "test")

	// Mock transaction creation that will fail
	jsonResponse, err := json.Marshal(upholdCreateTransactionFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
		),
		httpmock.NewStringResponder(500, string(jsonResponse)),
	)

	ctx := context.Background()
	mockDriver := new(mockDriver)
	transaction := Transaction{State: Authorized}
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	upholdStateMachine := UpholdMachine{}
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the meantime.
	// @TODO: Make this test fail
	// currentVersion := 500

	// Should fail to transition transaction into the Pending state
	newState, _ := Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Authorized, newState)
}

/*
TestUpholdStateMachine404FailureToPaidTransitions tests for a failure to progress status
Failure with 404 error when attempting to transfer from Pending to Paid
*/
func TestUpholdStateMachine404FailureToPaidTransitions(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	err := os.Setenv("UPHOLD_ENVIRONMENT", "test")
	if err != nil {
		panic("failed to set environment variable")
	}

	// Mock transaction commit that will fail
	jsonResponse, err := json.Marshal(upholdCommitTransactionFailureResponse)
	if err != nil {
		panic(err)
	}
	httpmock.RegisterResponder(
		"POST",
		fmt.Sprintf(
			"%s/v0/me/cards/%s/transactions/%s/commit",
			mockUpholdHost,
			upholdWallet.Info.ProviderID,
			geminiSucceedTransaction.ProviderID,
		),
		httpmock.NewStringResponder(404, string(jsonResponse)),
	)

	ctx := context.Background()
	transaction := Transaction{State: Pending}
	mockDriver := new(mockDriver)
	ctx = context.WithValue(ctx, ctxAuthKey{}, "some authorization from CLI")
	// When the implementation is in place, this Version value will not be necessary.
	// However, it's set here to allow the placeholder implementation to return the
	// correct value and allow this test to pass in the mean time.
	// @TODO: Make this test fail
	// currentVersion := 404
	upholdStateMachine := UpholdMachine{}

	// Should transition transaction into the Pending state
	newState, _ := Drive(ctx, &upholdStateMachine, &transaction, mockDriver)
	assert.Equal(t, Pending, newState)
}
