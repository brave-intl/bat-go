package payments

// QLDBPaymentTransitionState is an integer representing transaction status
type QLDBPaymentTransitionState int64

const (
	// Initialized represents the first state that a transaction record
	Initialized QLDBPaymentTransitionState = iota
	// Prepared represents a record that has been prepared for authorization
	Prepared
	// Authorized represents a record that has been authorized
	Authorized
	// Pending represents a record that is being or has been submitted to a processor
	Pending
	// Paid represents a record that has entered a finalzed success state with a processor
	Paid
	// Failed represents a record that has failed processing permanently
	Failed
)

// Transitions represents the valid forward-transitions for each given state
var Transitions = map[QLDBPaymentTransitionState][]QLDBPaymentTransitionState{
	Initialized: {Prepared, Failed},
	Prepared:    {Authorized, Failed},
	Authorized:  {Pending, Failed},
	Pending:     {Paid, Failed},
	Paid:        {},
	Failed:      {},
}

// GetValidTransitions returns valid transitions
func (q QLDBPaymentTransitionState) GetValidTransitions() []QLDBPaymentTransitionState {
	return Transitions[q]
}

// GetAllValidTransitionSequences returns all valid transition sequences
func GetAllValidTransitionSequences() [][]QLDBPaymentTransitionState {
	return recurseTransitionResolution(0, []QLDBPaymentTransitionState{})
}

func recurseTransitionResolution(
	state QLDBPaymentTransitionState,
	currentTree []QLDBPaymentTransitionState,
) [][]QLDBPaymentTransitionState {
	var (
		result      [][]QLDBPaymentTransitionState
		updatedTree = append(currentTree, state)
	)
	possibleStates := Transitions[state]
	if len(possibleStates) == 0 {
		tempTree := make([]QLDBPaymentTransitionState, len(updatedTree))
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
