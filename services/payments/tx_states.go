package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TransactionState is an integer representing transaction status.
type TransactionState string

// TxStateMachine describes types with the appropriate methods to be Driven as a state machine
const (
	// Prepared represents a record that has been prepared for authorization.
	Prepared TransactionState = "prepared"
	// Authorized represents a record that has been authorized.
	Authorized TransactionState = "authorized"
	// Pending represents a record that is being or has been submitted to a processor.
	Pending TransactionState = "pending"
	// Paid represents a record that has entered a finalized success state with a processor.
	Paid TransactionState = "paid"
	// Failed represents a record that has failed processing permanently.
	Failed TransactionState = "failed"
)

// Transitions represents the valid forward-transitions for each given state.
var Transitions = map[TransactionState][]TransactionState{
	Prepared:   {Authorized, Failed},
	Authorized: {Pending, Failed},
	Pending:    {Paid, Failed},
	Paid:       {},
	Failed:     {},
}

type baseStateMachine struct {
	transaction      *Transaction
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
	transaction *Transaction,
) {
	s.datastore = datastore
	s.sdkClient = sdkClient
	s.kmsSigningClient = kmsSigningClient
	s.kmsSigningKeyID = kmsSigningKeyID
	s.transaction = transaction
}

func (s *baseStateMachine) wrappedWrite(ctx context.Context) (*Transaction, error) {
	return WriteTransaction(
		ctx,
		s.datastore,
		s.sdkClient,
		s.kmsSigningClient,
		s.kmsSigningKeyID,
		s.transaction,
	)
}

func (s *baseStateMachine) writeNextState(ctx context.Context, nextState TransactionState) (*Transaction, error) {
	if !s.transaction.nextStateValid(nextState) {
		return nil, fmt.Errorf("invalid state transition from %s to %s for transaction %s", s.transaction.State, nextState, s.transaction.ID)
	}
	s.transaction.State = nextState
	entry, err := s.wrappedWrite(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to write transaction: %w", err)
	}
	s.transaction = entry
	return entry, nil
}

// Prepare implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Prepare(ctx context.Context) (*Transaction, error) {
	return s.writeNextState(ctx, Prepared)
}

// Authorize implements TxStateMachine for the baseStateMachine.
func (s *baseStateMachine) Authorize(ctx context.Context) (*Transaction, error) {
	return s.writeNextState(ctx, Authorized)
}

func (s *baseStateMachine) setTransaction(transaction *Transaction) {
	s.transaction = transaction
}

// GetState returns transaction state for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getState() TransactionState {
	return s.transaction.State
}

// GetTransaction returns a full transaction for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getTransaction() *Transaction {
	return s.transaction
}

// GetTransactionID returns a transaction id for a state machine, implementing TxStateMachine.
func (s *baseStateMachine) getTransactionID() *uuid.UUID {
	return s.transaction.ID
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
	return s.transaction.GenerateIdempotencyKey(namespace)
}

// StateMachineFromTransaction returns a state machine when provided a transaction.
func StateMachineFromTransaction(transaction *Transaction, service *Service) (TxStateMachine, error) {
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
) (*Transaction, error) {
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
func populateInitialTransaction[T TxStateMachine](ctx context.Context, machine T) (*Transaction, error) {
	namespace := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
	// Make sure the transaction we have has an ID that matches its contents before we check if it exists
	generatedID, err := machine.GenerateTransactionID(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to generate idempotency key for %s: %w", machine.getTransactionID(), err)
	}
	if generatedID != machine.getTransactionID() {
		return nil, errors.New("provided idempotencyKey does not match transaction")
	}
	// Check if the transaction exists so that we know whether to create it
	transaction, err := GetTransactionByID(
		ctx,
		machine.getDatastore(),
		machine.getSDKClient(),
		machine.getKMSSigningClient(),
		machine.getTransactionID(),
	)
	if err != nil {
		// If the transaction doesn't exist in the database, prepare it
		var notFound *QLDBReocrdNotFoundError
		if errors.As(err, &notFound) {
			return machine.Prepare(ctx)
		}
		return nil, fmt.Errorf("failed to get transaction from QLDB: %w", err)
	}
	return nil, fmt.Errorf("transaction %s already exists in QLDB and cannot be prepared", transaction.ID)
}

// GetValidTransitions returns valid transitions.
func (ts TransactionState) GetValidTransitions() []TransactionState {
	return Transitions[ts]
}

// GetAllValidTransitionSequences returns all valid transition sequences.
func GetAllValidTransitionSequences() [][]TransactionState {
	return recurseTransitionResolution("prepared", []TransactionState{})
}

// recurseTransitionResolution returns the list of valid transition paths that are
// possible for a given state.
func recurseTransitionResolution(
	state TransactionState,
	currentTree []TransactionState,
) [][]TransactionState {
	var (
		result      [][]TransactionState
		updatedTree = append(currentTree, state)
	)
	possibleStates := state.GetValidTransitions()
	if len(possibleStates) == 0 {
		tempTree := make([]TransactionState, len(updatedTree))
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
