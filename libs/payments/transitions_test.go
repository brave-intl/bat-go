package payments

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

/*
Generate all valid transition sequences and ensure that this test contains the exact same set of
valid transition sequences. The purpose of this test is to alert us if outside changes
impact the set of valid transitions.
*/
func TestRecurseTransitionResolution(t *testing.T) {
	allValidTransitionSequences := RecurseTransitionResolution("prepared", []PaymentStatus{})
	knownValidTransitionSequences := [][]PaymentStatus{
		{Prepared, Authorized, Pending, Paid},
		{Prepared, Authorized, Pending, Failed},
		{Prepared, Authorized, Failed},
		{Prepared, Failed},
	}
	// Ensure all generatedTransitionSequence have a matching knownValidTransitionSequences
	for _, generatedTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, knownValidTransitionSequence := range knownValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		assert.True(t, foundMatch)
	}
	// Ensure all knownValidTransitionSequences have a matching generatedTransitionSequence
	for _, knownValidTransitionSequence := range allValidTransitionSequences {
		foundMatch := false
		for _, generatedTransitionSequence := range allValidTransitionSequences {
			if reflect.DeepEqual(generatedTransitionSequence, knownValidTransitionSequence) {
				foundMatch = true
			}
		}
		assert.True(t, foundMatch)
	}
}
