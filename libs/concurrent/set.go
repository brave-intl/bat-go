package concurrent

import "sync"

// Set implements a concurrent set of keys
type Set struct {
	m map[string]bool
	sync.RWMutex
}

// NewSet creates a new Set
func NewSet() *Set {
	return &Set{
		m: make(map[string]bool),
	}
}

// Add the key to the set, returns true if the key did not already exist
func (s *Set) Add(k string) bool {
	s.Lock()
	defer s.Unlock()
	_, exists := s.m[k]
	s.m[k] = true
	return !exists
}

// Remove the key from the set
func (s *Set) Remove(k string) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, k)
}
