package cryptography

import (
	"crypto/hmac"
	"crypto/sha512"
	"errors"
)

// HMACKey an interface for hashing to hmac-sha384
type HMACKey interface {
	// HMACSha384 does the appropriate hashing
	HMACSha384(payload []byte) ([]byte, error)
}

// HMACHasher is an in process signer implementation for HMACKey
type HMACHasher struct {
	secret []byte
}

// NewHMACHasher creates a new HMACKey for hashing
func NewHMACHasher(secret []byte) HMACKey {
	hasher := HMACHasher{secret}
	return &hasher
}

// HMACSha384 hashes using an in process secret
func (hmh *HMACHasher) HMACSha384(payload []byte) ([]byte, error) {
	mac := hmac.New(sha512.New384, hmh.secret)
	len, err := mac.Write([]byte(payload))
	if err != nil {
		return []byte{}, err
	}
	if len == 0 {
		return []byte{}, errors.New("no bytes written in HMACSha384 Hash")
	}
	return mac.Sum(nil), nil
}
