package httpsignature

import (
	"crypto"
	"encoding/hex"
	"errors"
	"io"
	"strconv"

	"golang.org/x/crypto/ed25519"
)

// Ed25519PubKey a wrapper type around ed25519.PublicKey to fulfill interface Verifier
type Ed25519PubKey ed25519.PublicKey

// Verify the signature sig for message using the ed25519 public key pk
// Returns true if the signature is valid, false if not and error if the key provided is not valid
func (pk Ed25519PubKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	if l := len(pk); l != ed25519.PublicKeySize {
		return false, errors.New("ed25519: bad public key length: " + strconv.Itoa(l))
	}

	return ed25519.Verify(ed25519.PublicKey(pk), message, sig), nil
}

func (pk Ed25519PubKey) String() string {
	return hex.EncodeToString(pk)
}

// GenerateEd25519Key generate an ed25519 keypair and return it
func GenerateEd25519Key(rand io.Reader) (Ed25519PubKey, ed25519.PrivateKey, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	return Ed25519PubKey(publicKey), privateKey, err
}

func GetEd25519KeyId(key ed25519.PrivateKey) string {
	pubkey := key.Public().(ed25519.PublicKey)
	return hex.EncodeToString(pubkey)
}

func GetEd25519RequestSignator(key ed25519.PrivateKey) ParameterizedSignator {
	return ParameterizedSignator{
		SignatureParams: SignatureParams{
			Algorithm: ED25519,
			KeyID:     GetEd25519KeyId(key),
			Headers:   RequestSigningHeaders,
		},
		Signator: key,
	}
}
