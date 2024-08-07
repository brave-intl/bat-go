package httpsignature

import (
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
)

// Ed25519PubKey a wrapper type around ed25519.PublicKey to fulfill interface Verifier
type Ed25519PubKey ed25519.PublicKey

// Ed25519PubKey a wrapper type around ed25519.PublicKey to fulfill interface Signator
type Ed25519PrivKey ed25519.PrivateKey

// ED25519-specific signator with access to the corresponding public key
type Ed25519Signator interface {
	Signator
	Public() Ed25519PubKey
}

// Verify the signature sig for message using the ed25519 public key pk
// Returns true if the signature is valid, false if not and error if the key provided is not valid
func (pk Ed25519PubKey) VerifySignature(message, sig []byte) error {
	if l := len(pk); l != ed25519.PublicKeySize {
		return fmt.Errorf("ed25519: bad public key length: %d", l)
	}

	if !ed25519.Verify(ed25519.PublicKey(pk), message, sig) {
		return ErrBadSignature
	}
	return nil
}

func (pk Ed25519PubKey) String() string {
	return hex.EncodeToString(pk)
}

func (privKey Ed25519PrivKey) SignMessage(
	message []byte,
) (signature []byte, err error) {
	return ed25519.PrivateKey(privKey).Sign(nil, message, crypto.Hash(0))
}

func (privKey Ed25519PrivKey) Public() Ed25519PubKey {
	pubKey := ed25519.PrivateKey(privKey).Public().(ed25519.PublicKey)
	return Ed25519PubKey(pubKey)
}

// Get the public key encoded as hexadecimal string
func (privKey Ed25519PrivKey) PublicHex() string {
	pubKey := ed25519.PrivateKey(privKey).Public().(ed25519.PublicKey)
	return hex.EncodeToString(pubKey)
}

// GenerateEd25519Key generate an ed25519 private key
func GenerateEd25519Key() (Ed25519PrivKey, error) {
	_, privateKey, err := ed25519.GenerateKey(nil)
	return Ed25519PrivKey(privateKey), err
}
