package kv

import "errors"

// An unsafe key-value datastore implementation using a map for testing
type UnsafeMapKv struct {
	m map[string]string
}

func NewUnsafe() *UnsafeMapKv {
	return &UnsafeMapKv{map[string]string{}}
}

// NOTE ttl is ignored for this implementation
func (m *UnsafeMapKv) Set(key string, value string, ttl int, upsert bool) (bool, error) {
	if _, ok := m.m[key]; !ok || upsert {
		m.m[key] = value
		return true, nil
	}
	return false, errors.New("Set failed")
}

func (m *UnsafeMapKv) Get(key string) (string, error) {
	if value, ok := m.m[key]; ok {
		return value, nil
	}
	return "", errors.New("Get failed")
}

func (m *UnsafeMapKv) Delete(key string) (bool, error) {
	if _, ok := m.m[key]; ok {
		delete(m.m, key)
		return true, nil
	}
	return false, nil
}

func (m *UnsafeMapKv) Close() error {
	return nil
}
