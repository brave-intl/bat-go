package httpsignature

import (
	"crypto"
	"errors"
	"golang.org/x/crypto/ed25519"
	"strconv"
)

type Ed25519PubKey ed25519.PublicKey

func (pk Ed25519PubKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	if l := len(pk); l != ed25519.PublicKeySize {
		return false, errors.New("ed25519: bad public key length: " + strconv.Itoa(l))
	}

	key := make([]byte, ed25519.PublicKeySize)
	copy(key, pk)

	return ed25519.Verify(key, message, sig), nil
}
