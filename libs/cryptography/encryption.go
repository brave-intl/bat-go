package cryptography

import (
	"crypto/rand"
	"errors"
	"io"

	"golang.org/x/crypto/nacl/secretbox"
)

// Both what the encryption key length should be
var keyLength = 32

// 4KB is a reasonable chunk size.
const secretBoxMaxChunkSize = 4000

var (
	// ErrEncryptedFieldTooLarge - the sku was invalid
	ErrEncryptedFieldTooLarge = errors.New("encrypted field is greater than 4 KB - this must be chunked")
)

// EncryptMessage uses SecretBox to encrypt the message
func EncryptMessage(encryptionKey [32]byte, field []byte) (encrypted []byte, nonceString [24]byte, err error) {
	var nonce [24]byte

	// The key argument should be 32 bytes long
	if len(encryptionKey) != keyLength {
		return nil, nonce, errors.New("encryption Key is not the correct key length")
	}

	// large amounts of data should be chunked
	if len(field) >= secretBoxMaxChunkSize {
		return nil, nonce, ErrEncryptedFieldTooLarge
	}

	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, nonce, err
	}

	out := make([]byte, 0, secretbox.Overhead+len(field))
	encryptedField := secretbox.Seal(out[:], field, &nonce, &encryptionKey)

	return encryptedField, nonce, nil
}

// DecryptMessage uses SecretBox to decrypt the message
func DecryptMessage(encryptionKey [32]byte, encryptedField []byte, nonce []byte) (string, error) {
	var decryptNonce [24]byte
	copy(decryptNonce[:], nonce)
	decrypted, ok := secretbox.Open(nil, encryptedField, &decryptNonce, &encryptionKey)
	if !ok {
		return "", errors.New("could not decrypt the value of the secret")
	}

	return string(decrypted), nil
}
