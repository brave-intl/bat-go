package set

// Package set includes two set implementations, both backed by slices, one of
// which is safe. Intended for testing purposes

import (
	"sync"
)

// UnsafeSliceSet implements an unsafe slice backed set
type UnsafeSliceSet struct {
	slice []string
}

// NewUnsafeSliceSet creates a new UnsafeSliceSet
func NewUnsafeSliceSet() UnsafeSliceSet {
	return UnsafeSliceSet{}
}

// Cardinality returns the number of elements in the set
func (set *UnsafeSliceSet) Cardinality() (int, error) {
	return len(set.slice), nil
}

// Contains returns true if the given item is in the set
func (set *UnsafeSliceSet) Contains(e string) (bool, error) {
	for _, a := range set.slice {
		if a == e {
			return true, nil
		}
	}
	return false, nil
}

// Add a single element to the set, return true if newly added
func (set *UnsafeSliceSet) Add(e string) (bool, error) {
	r, err := set.Contains(e)
	if err != nil {
		panic(err)
	}
	if r {
		return false, err
	}
	set.slice = append(set.slice, e)
	return true, nil
}

// Close the underlying connection to the datastore
func (set *UnsafeSliceSet) Close() error {
	return nil
}

// SliceSet implements an safe slice backed set
type SliceSet struct {
	u *UnsafeSliceSet
	sync.RWMutex
}

// NewSliceSet creates a new SliceSet
func NewSliceSet() SliceSet {
	tmp := NewUnsafeSliceSet()
	return SliceSet{u: &tmp}
}

// Cardinality returns the number of elements in the set
func (set *SliceSet) Cardinality() (int, error) {
	set.RLock()
	defer set.RUnlock()
	return set.u.Cardinality()
}

// Contains returns true if the given item is in the set
func (set *SliceSet) Contains(e string) (bool, error) {
	set.RLock()
	defer set.RUnlock()
	return set.u.Contains(e)
}

// Add a single element to the set, return true if newly added
func (set *SliceSet) Add(e string) (bool, error) {
	set.Lock()
	ret, err := set.u.Add(e)
	if err != nil {
		panic(err)
	}
	set.Unlock()
	return ret, nil
}

// Close the underlying connection to the datastore
func (set *SliceSet) Close() error {
	return nil
}
