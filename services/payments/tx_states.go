package payments

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/brave-intl/bat-go/libs/payments"
)

type baseStateMachine struct {
	transaction      *AuthenticatedPaymentState
	datastore        wrappedQldbDriverAPI
	sdkClient        wrappedQldbSDKClient
	kmsSigningClient wrappedKMSClient
	kmsSigningKeyID  string
}

func (s *baseStateMachine) setPersistenceConfigValues(
	datastore wrappedQldbDriverAPI,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	kmsSigningKeyID string,
	transaction *AuthenticatedPaymentState,
) {
	s.datastore = datastore
	s.sdkClient = sdkClient
	s.kmsSigningClient = kmsSigningClient
	s.kmsSigningKeyID = kmsSigningKeyID
	s.transaction = transaction
}

func (s *baseStateMachine) wrappedWrite(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return writeTransaction(
		ctx,
		s.datastore,
		s.sdkClient,
		s.kmsSigningClient,
		s.kmsSigningKeyID,
		s.transaction,
	)
}

func (s *baseStateMachine) writeNextState(
	ctx context.Context,
	nextState PaymentStatus,
) (*AuthenticatedPaymentState, error) {
	if !s.transaction.NextStateValid(nextState) {
		return nil, fmt.Errorf(
			"invalid state transition from %s to %s for transaction %s",
			s.transaction.Status,
			nextState,
			s.transaction.DocumentID,
		)
	}
	s.transaction.Status = nextState
	authenticatedState, err := s.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	s.transaction = authenticatedState
	return authenticatedState, nil
}

// Prepare implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Prepare(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.writeNextState(ctx, Prepared)
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Authorize(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return s.writeNextState(ctx, Authorized)
}

func (s *baseStateMachine) setTransaction(transaction *AuthenticatedPaymentState) {
	s.transaction = transaction
}

// GetState returns transaction state for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getState() PaymentStatus {
	return s.transaction.Status
}

// GetTransaction returns a full transaction for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getTransaction() *AuthenticatedPaymentState {
	return s.transaction
}

// getDatastore returns a transaction id for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getDatastore() wrappedQldbDriverAPI {
	return s.datastore
}

// getSDKClient returns a transaction id for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getSDKClient() wrappedQldbSDKClient {
	return s.sdkClient
}

// getKMSSigningClient returns a transaction id for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getKMSSigningClient() wrappedKMSClient {
	return s.kmsSigningClient
}

// GenerateTransactionID returns the generated transaction id for a state machine's transaction,
// implementing TxStateMachine.
func (s *baseStateMachine) GenerateTransactionID() (*uuid.UUID, error) {
	paymentStateID := s.transaction.GenerateIdempotencyKey()
	return &paymentStateID, nil
}

// StateMachineFromTransaction returns a state machine when provided a transaction.
func StateMachineFromTransaction(
	service *Service,
	authenticatedState *AuthenticatedPaymentState,
) (TxStateMachine, error) {
	var machine TxStateMachine

	switch authenticatedState.PaymentDetails.Custodian {
	case "uphold":
		machine = &UpholdMachine{}
	case "bitflyer":
		// Set Bitflyer-specific properties
		machine = &BitflyerMachine{
			client: *http.DefaultClient,
			bitflyerHost: os.Getenv("BITFLYER_SERVER"),
		}
	case "gemini":
		machine = &GeminiMachine{}
	}
	machine.setPersistenceConfigValues(
		service.datastore,
		service.sdkClient,
		service.kmsSigningClient,
		service.kmsSigningKeyID,
		authenticatedState,
	)
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*AuthenticatedPaymentState, error) {
	// Drive is called recursively, so we need to check whether a deadline has been set
	// by a prior caller and only set the default deadline if not.
	if _, deadlineSet := ctx.Deadline(); !deadlineSet {
		ctx, _ = context.WithTimeout(ctx, 5 * time.Minute)
	}
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		transaction := machine.getTransaction()
		return transaction, err
	}

	// If the transaction does exist in the database, attempt to drive the state machine forward
	switch machine.getState() {
	case Prepared:
		//if len(machine.getTransaction().Authorizations) >= 3 /* TODO MIN AUTHORIZERS */ {
			return machine.Authorize(ctx)
		//}
		//return nil, &InsufficientAuthorizationsError{}
	case Authorized:
		return machine.Pay(ctx)
	case Pending:
		return machine.Pay(ctx)
	case Paid:
		return machine.Pay(ctx)
	case Failed:
		return machine.Fail(ctx)
	default:
		return nil, errors.New("invalid transition state")
	}
}
