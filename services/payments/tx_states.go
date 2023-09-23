package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	. "github.com/brave-intl/bat-go/libs/payments"
)

type baseStateMachine struct {
	idempotencyKey   uuid.UUID
	transaction      *AuthenticatedPaymentState
	datastore        wrappedQldbDriverAPI
	sdkClient        wrappedQldbSDKClient
	kmsSigningClient wrappedKMSClient
	kmsSigningKeyID  string
}

func (s *baseStateMachine) setPersistenceConfigValues(
	idempotencyKey uuid.UUID,
	datastore wrappedQldbDriverAPI,
	sdkClient wrappedQldbSDKClient,
	kmsSigningClient wrappedKMSClient,
	kmsSigningKeyID string,
	transaction *AuthenticatedPaymentState,
) {
	s.idempotencyKey = idempotencyKey
	s.datastore = datastore
	s.sdkClient = sdkClient
	s.kmsSigningClient = kmsSigningClient
	s.kmsSigningKeyID = kmsSigningKeyID
	s.transaction = transaction
}

func (s *baseStateMachine) wrappedWrite(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return WriteTransaction(
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
	entry, err := s.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	s.transaction = entry
	return entry, nil
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

// getTransactionID returns a transaction id for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getIdempotencyKey() uuid.UUID {
	return s.idempotencyKey
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
func (s *baseStateMachine) GenerateTransactionID(namespace uuid.UUID) (*uuid.UUID, error) {
	paymentStateID := s.transaction.GenerateIdempotencyKey(namespace)
	return &paymentStateID, nil
}

// StateMachineFromTransaction returns a state machine when provided a transaction.
func StateMachineFromTransaction(
	id uuid.UUID,
	transaction *AuthenticatedPaymentState,
	service *Service,
) (TxStateMachine, error) {
	var machine TxStateMachine

	switch transaction.Custodian {
	case "uphold":
		machine = &UpholdMachine{}
	case "bitflyer":
		machine = &BitflyerMachine{}
	case "gemini":
		machine = &GeminiMachine{}
	}
	machine.setPersistenceConfigValues(
		id,
		service.datastore,
		service.sdkClient,
		service.kmsSigningClient,
		service.kmsSigningKeyID,
		transaction,
	)
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*AuthenticatedPaymentState, error) {
	// If the transaction does exist in the database, attempt to drive the state machine forward
	switch machine.getState() {
	case Prepared:
		if len(machine.getTransaction().Authorizations) >= 3 /* TODO MIN AUTHORIZERS */ {
			return machine.Authorize(ctx)
		}
		return nil, &InsufficientAuthorizationsError{}
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

// populateInitialTransaction creates the transaction in the database and calls the Prepare
// method on it. When creating the initial object in the database for a transaction we must
// verify that the transaction does not already exist. Once a transaction exists, we alawys
// refer to it by DocumentID and progress it with Drive.
func populateInitialTransaction[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*AuthenticatedPaymentState, error) {
	namespace := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
	// Make sure the transaction we have has an ID that matches its contents before we check if it
	// exists
	generatedID, err := machine.GenerateTransactionID(namespace)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to generate idempotency key for %s: %w",
			machine.getIdempotencyKey(),
			err,
		)
	}

	if generatedID.String() != machine.getIdempotencyKey().String() {
		return nil, fmt.Errorf(
			"provided idempotencyKey does not match the generated key: %s %s",
			generatedID.String(),
			machine.getIdempotencyKey().String(),
		)
	}
	// Check if the transaction exists so that we know whether to create it
	_, err = GetTransactionByIdempotencyKey(
		ctx,
		machine.getDatastore(),
		machine.getSDKClient(),
		machine.getKMSSigningClient(),
		machine.getIdempotencyKey(),
	)
	if err != nil {
		// If the transaction doesn't exist in the database, prepare it
		var notFound *QLDBReocrdNotFoundError
		if errors.As(err, &notFound) {
			return machine.Prepare(ctx)
		}
		return nil, fmt.Errorf("failed to check for the existence of transaction in QLDB: %w", err)
	}
	return nil, fmt.Errorf(
		"transaction %s already exists in QLDB and cannot be prepared",
		generatedID,
	)
}

// GetAllValidTransitionSequences returns all valid transition sequences.
func GetAllValidTransitionSequences() [][]PaymentStatus {
	return recurseTransitionResolution("prepared", []PaymentStatus{})
}

// recurseTransitionResolution returns the list of valid transition paths that are
// possible for a given state.
func recurseTransitionResolution(
	state PaymentStatus,
	currentTree []PaymentStatus,
) [][]PaymentStatus {
	var (
		result      [][]PaymentStatus
		updatedTree = append(currentTree, state)
	)
	possibleStates := state.GetValidTransitions()
	if len(possibleStates) == 0 {
		tempTree := make([]PaymentStatus, len(updatedTree))
		copy(tempTree, updatedTree)
		result = append(result, tempTree)
		return result
	}
	for _, possibleState := range possibleStates {
		recursed := recurseTransitionResolution(possibleState, updatedTree)
		result = append(result, recursed...)
	}
	return result
}
