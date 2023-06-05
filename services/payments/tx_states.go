package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TransactionState is an integer representing transaction status
type TransactionState string

// TxStateMachine describes types with the appropriate methods to be Driven as a state machine
const (
	// Prepared represents a record that has been prepared for authorization
	Prepared TransactionState = "prepared"
	// Authorized represents a record that has been authorized
	Authorized TransactionState = "authorized"
	// Pending represents a record that is being or has been submitted to a processor
	Pending TransactionState = "pending"
	// Paid represents a record that has entered a finalized success state with a processor
	Paid TransactionState = "paid"
	// Failed represents a record that has failed processing permanently
	Failed TransactionState = "failed"
)

// Transitions represents the valid forward-transitions for each given state
var Transitions = map[TransactionState][]TransactionState{
	Prepared:   {Authorized, Failed},
	Authorized: {Pending, Failed},
	Pending:    {Paid, Failed},
	Paid:       {},
	Failed:     {},
}

// StateMachineFromTransaction returns a state machine when provided a transaction
func StateMachineFromTransaction(transaction *Transaction, service *Service) (TxStateMachine, error) {
	var machine TxStateMachine
	switch transaction.Custodian {
	case "uphold":
		machine = NewUpholdMachine(transaction, service)
	case "bitflyer":
		machine = NewBitflyerMachine(transaction, service)
	case "gemini":
		machine = NewGeminiMachine(transaction, service)
	}
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
) (*Transaction, error) {
	namespace := ctx.Value(serviceNamespaceContextKey{}).(uuid.UUID)
	// Make sure the transaction we have has an ID that matches its contents before we check if it exists
	generatedID, err := machine.GenerateTransactionID(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to generate idempotency key for %s: %w", machine.GetTransactionID(), err)
	}
	if generatedID != machine.GetTransactionID() {
		return nil, errors.New("provided idempotencyKey does not match transaction")
	}
	// Check if the transaction exists so that we know whether to create it or progress it
	transaction, err := machine.GetService().GetTransactionByID(ctx, generatedID)
	if err != nil {
		// If the transaction doesn't exist in the database, prepare it
		var notFound *QLDBReocrdNotFoundError
		if errors.As(err, &notFound) {
			return machine.Prepare(ctx)
		}
		return nil, fmt.Errorf("failed to get transaction from QLDB: %w", err)
	}
	// Set the machine's transaction to the values retrieved from the database. This helps avoid cases where the State
	// in the transaction provided by the client is out of date with the database
	machine.setTransaction(transaction)
	// If the transaction does exist in the database, attempt to drive the state machine forward
	switch machine.GetState() {
	case Prepared:
		return machine.Authorize(ctx)
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

// GetValidTransitions returns valid transitions
func (ts TransactionState) GetValidTransitions() []TransactionState {
	return Transitions[ts]
}

// GetAllValidTransitionSequences returns all valid transition sequences
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
