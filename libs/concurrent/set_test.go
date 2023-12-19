package concurrent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSet(t *testing.T) {
	k := "key"
	s := NewSet()
	// First insert returns true
	assert.Equal(t, true, s.Add(k))
	// Subsequent insert(s) returns false
	assert.Equal(t, false, s.Add(k))
	assert.Equal(t, false, s.Add(k))
	// We can delete a key from the set
	s.Remove(k)
	// First insert returns true
	assert.Equal(t, true, s.Add(k))
}
