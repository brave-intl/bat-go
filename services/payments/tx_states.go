package payments

import (
	"context"
	"errors"
)

// TransactionState is an integer representing transaction status
type TransactionState int64

// TxStateMachine describes types with the appropriate methods to be Driven as a state machine
const (
	// Initialized represents the first state that a transaction record
	Initialized TransactionState = iota
	// Prepared represents a record that has been prepared for authorization
	Prepared
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
	Initialized: {Prepared, Failed},
	Prepared:    {Authorized, Failed},
	Authorized:  {Pending, Failed},
	Pending:     {Paid, Failed},
	Paid:        {},
	Failed:      {},
}

func StateMachineFromTransaction(transaction *Transaction) (TxStateMachine, error) {
	var machine TxStateMachine
	switch transaction.Custodian {
	case "uphold":
		machine = &UpholdMachine{}
	case "bitflyer":
		machine = &BitflyerMachine{}
	case "gemini":
		machine = &GeminiMachine{}
	}
	return machine, nil
}

// Drive switches on the provided currentTransactionState and executes the appropriate
// method from the provided TxStateMachine to attempt to progress the state.
func Drive[T TxStateMachine](
	ctx context.Context,
	machine T,
	transaction *Transaction,
	connection wrappedQldbDriverAPI,
) (TransactionState, error) {
	machine.setTransaction(transaction)
	machine.setConnection(connection)
	switch transaction.State {
	case Initialized:
		return machine.Initialized()
	case Prepared:
		return machine.Prepared()
	case Authorized:
		return machine.Authorized()
	case Pending:
		return machine.Pending()
	case Paid:
		return machine.Paid()
	case Failed:
		return machine.Failed()
	default:
		return Initialized, errors.New("invalid transition state")
	}
}

// GetValidTransitions returns valid transitions
func (ts TransactionState) GetValidTransitions() []TransactionState {
	return Transitions[ts]
}

// String implements ToString for TransactionState
func (ts TransactionState) String() string {
	switch ts {
	case Initialized:
		return "initialized"
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
	possibleStates := Transitions[state]
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
