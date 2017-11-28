package set

import (
	"sync"
)

type UnsafeSliceSet struct {
	slice []string
}

func NewUnsafeSliceSet() UnsafeSliceSet {
	return UnsafeSliceSet{}
}

func (set *UnsafeSliceSet) Cardinality() (int, error) {
	return len(set.slice), nil
}

func (set *UnsafeSliceSet) Contains(e string) (bool, error) {
	for _, a := range set.slice {
		if a == e {
			return true, nil
		}
	}
	return false, nil
}

func (set *UnsafeSliceSet) Add(e string) (bool, error) {
	if r, _ := set.Contains(e); r {
		return false, nil
	}
	set.slice = append(set.slice, e)
	return true, nil
}

func (set *UnsafeSliceSet) Close() error {
	return nil
}

type SliceSet struct {
	u *UnsafeSliceSet
	sync.RWMutex
}

func NewSliceSet() SliceSet {
	tmp := NewUnsafeSliceSet()
	return SliceSet{u: &tmp}
}

func (set *SliceSet) Cardinality() (int, error) {
	set.RLock()
	defer set.RUnlock()
	return set.u.Cardinality()
}

func (set *SliceSet) Contains(e string) (bool, error) {
	set.RLock()
	defer set.RUnlock()
	return set.u.Contains(e)
}

func (set *SliceSet) Add(e string) (bool, error) {
	set.Lock()
	ret, _ := set.u.Add(e)
	set.Unlock()
	return ret, nil
}

func (set *SliceSet) Close() error {
	return nil
}
