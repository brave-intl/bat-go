// Package test provides utilities for testing. Do not import this into non-test code.
package test

import (
	"crypto/rand"
	"math"
	"math/big"
)

// RandomString return a random alphanumeric string with length 10.
func RandomString() string {
	return RandomStringWithLen(10)
}

// RandomStringWithLen returns a random alphanumeric string with a specified length.
func RandomStringWithLen(length int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	s := make([]rune, length)
	for i := range s {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		s[i] = letters[n.Int64()]
	}
	return string(s)
}

// RandomInt return a random int up to math.MaxInt32.
func RandomInt() int {
	return RandomIntWithMax(math.MaxInt32)
}

// RandomIntWithMax returns a random int in range [0, max].
func RandomIntWithMax(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	i := n.Int64()
	if i == 0 {
		i = 1
	}
	return int(i)
}

// RandomNonZeroInt return a random nonzero int up to the supplied max.
func RandomNonZeroInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)-1))
	return int(n.Int64() + 1)
}
