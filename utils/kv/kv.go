// Package kv contains "key-value" stores used for testing
package kv

import "errors"

// UnsafeMapKv is an unsafe key-value store implementation using a map for testing
type UnsafeMapKv struct {
	m map[string]string
}

// NewUnsafe creates a new UnsafeMapKv
func NewUnsafe() *UnsafeMapKv {
	return &UnsafeMapKv{m: map[string]string{}}
}

// Set the key to value, updating possible existing if upsert is true
// NOTE ttl is ignored for this implementation
func (m *UnsafeMapKv) Set(key string, value string, ttl int, upsert bool) (bool, error) {
	if _, ok := m.m[key]; !ok || upsert {
		m.m[key] = value
		return true, nil
	}
	return false, errors.New("Set failed")
}

// Get the value at key
func (m *UnsafeMapKv) Get(key string) (string, error) {
	if value, ok := m.m[key]; ok {
		return value, nil
	}
	return "", errors.New("Get failed")
}

// Delete the value at key
func (m *UnsafeMapKv) Delete(key string) (bool, error) {
	if _, ok := m.m[key]; ok {
		delete(m.m, key)
		return true, nil
	}
	return false, nil
}

// Close the store (no-op)
func (m *UnsafeMapKv) Close() error {
	return nil
}
