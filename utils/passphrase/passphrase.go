// Package passphrase implements passphrase based signing key derivation / backup from github.com/brave/crypto.
package passphrase

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/superp00t/niceware"
	bip39 "github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
)

var (
	// LedgerHKDFSalt from browser for deriving anonymous wallet signing key.
	// NOTE do not reuse this for other purposes, generate a new salt for new uses.
	LedgerHKDFSalt = []byte{126, 244, 99, 158, 51, 68, 253, 80, 133, 183, 51, 180, 77, 62, 74, 252, 62, 106, 96, 125, 241, 110, 134, 87, 190, 208, 158, 84, 125, 69, 246, 207, 162, 247, 107, 172, 37, 34, 53, 246, 105, 20, 215, 5, 248, 154, 179, 191, 46, 17, 6, 72, 210, 91, 10, 169, 145, 248, 22, 147, 117, 24, 105, 12}
)

// DeriveSigningKeysFromSeed using optional salt.
func DeriveSigningKeysFromSeed(seed, salt []byte) (ed25519.PrivateKey, error) {
	// NOTE info as []byte{0} not nil
	hkdf := hkdf.New(sha512.New, seed, salt, []byte{0})

	key := make([]byte, ed25519.SeedSize)
	_, err := io.ReadFull(hkdf, key)
	if err != nil {
		return key, err
	}

	return ed25519.NewKeyFromSeed(key), nil
}

// FromBytes converts bytes to passphrase using bip39.
func FromBytes(in []byte) ([]string, error) {
	phrase, err := bip39.NewMnemonic(in)
	return strings.Fields(phrase), err
}

// FromHex converts hex bytes to passphrase using bip39.
func FromHex(in string) ([]string, error) {
	b, err := hex.DecodeString(in)
	if err != nil {
		return nil, err
	}
	return FromBytes(b)
}

// ToBytes32 converts a 32-byte passphrase to bytes.
// Infers whether the passphrase is bip39 or niceware based on length.
func ToBytes32(phrase string) ([]byte, error) {
	words := strings.Fields(phrase)
	if len(words) == 16 {
		return niceware.PassphraseToBytes(words)
	} else if len(words) == 24 {
		return bip39.EntropyFromMnemonic(phrase)
	}
	return nil, fmt.Errorf("input words length %d is not 24 or 16", len(words))
}

// ToHex32 converts a 32-byte passphrase to hex.
// Infers whether the passphrase is bip39 or niceware based on length.
func ToHex32(phrase string) (string, error) {
	bytes, err := ToBytes32(phrase)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
