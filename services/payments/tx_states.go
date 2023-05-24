package payments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// TransactionState is an integer representing transaction status
type TransactionState int64

// TxStateMachine describes types with the appropriate methods to be Driven as a state machine
const (
	// Prepared represents a record that has been prepared for authorization
	Prepared TransactionState = iota
	// Authorized represents a record that has been authorized
	Authorized
	// Pending represents a record that is being or has been submitted to a processor
	Pending
	// Paid represents a record that has entered a finalized success state with a processor
	Paid
	// Failed represents a record that has failed processing permanently
	Failed
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
		return nil, fmt.Errorf("failed to get transaction from QLDB: %w", err)
	}
	// If the transaction doesn't exist in the database, prepare it
	if transaction == nil {
		return machine.Prepare(ctx)
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

// String implements ToString for TransactionState
func (ts TransactionState) String() string {
	switch ts {
	case Prepared:
		return "prepared"
	case Authorized:
		return "authorized"
	case Pending:
		return "pending"
	case Paid:
		return "paid"
	case Failed:
		return "failed"
	}
	return ""
}

// MarshalJSON implements JSON marshal for TransactionState
func (ts TransactionState) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", ts.String())), nil
}

// UnmarshalJSON implements JSON unmarshal for TransactionState
func (ts *TransactionState) UnmarshalJSON(data []byte) error {
	stringData := string(data)
	// Ignore null
	if stringData == "null" || stringData == `""` {
		return nil
	}
	switch stringData {
	case `"prepared"`:
		*ts = Prepared
	case `"authorized"`:
		*ts = Authorized
	case `"pending"`:
		*ts = Pending
	case `"paid"`:
		*ts = Paid
	case `"failed"`:
		*ts = Failed
	default:
		return errors.New("cannot unmarshal unknown transition state")
	}
	return nil
}

// GetAllValidTransitionSequences returns all valid transition sequences
func GetAllValidTransitionSequences() [][]TransactionState {
	return recurseTransitionResolution(0, []TransactionState{})
}

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
