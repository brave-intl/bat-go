package payments

// PaymentStatus is an integer representing transaction status.
type PaymentStatus string

const (
	// Prepared represents a record that has been prepared for authorization.
	Prepared PaymentStatus = "prepared"
	// Authorized represents a record that has been authorized.
	Authorized PaymentStatus = "authorized"
	// Pending represents a record that is being or has been submitted to a processor.
	Pending PaymentStatus = "pending"
	// Paid represents a record that has entered a finalized success state with a processor.
	Paid PaymentStatus = "paid"
	// Failed represents a record that has failed processing permanently.
	Failed PaymentStatus = "failed"
)

// Transitions represents the valid forward-transitions for each given state.
var Transitions = map[PaymentStatus][]PaymentStatus{
	Prepared:   {Authorized, Failed},
	Authorized: {Pending, Failed},
	Pending:    {Paid, Failed},
	Paid:       {},
	Failed:     {},
}

// GetValidTransitions returns valid transitions.
func (ts PaymentStatus) GetValidTransitions() []PaymentStatus {
	return Transitions[ts]
}

// GetAllValidTransitionSequences returns all valid transition sequences.
func GetAllValidTransitionSequences() [][]PaymentStatus {
	return RecurseTransitionResolution("prepared", []PaymentStatus{})
}

// RecurseTransitionResolution returns the list of valid transition paths that are
// possible for a given state.
func RecurseTransitionResolution(
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
		recursed := RecurseTransitionResolution(possibleState, updatedTree)
		result = append(result, recursed...)
	}
	return result
}
