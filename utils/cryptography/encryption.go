package crytography

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
)

// EncryptionKey for encrypting secrets
var EncryptionKey, _ = base64.StdEncoding.DecodeString(os.Getenv("ENCRYPTION_KEY"))

// Both what the encryption key length should be
var keyLength = 32

var (
	// ErrEncryptedFieldTooLarge - the sku was invalid
	ErrEncryptedFieldTooLarge = errors.New("Encrypted field is greater than 16 KB - this must be chunked")
)

// EncryptMessage uses SecretBox to encrypt the message
func EncryptMessage(field []byte) (encrypted []byte, nonceString [24]byte, err error) {
	var nonce [24]byte

	// The key argument should be 32 bytes long
	if len(EncryptionKey) != keyLength {
		return nil, nonce, errors.New("Encryption Key is not the correct key length")
	}

	// large amounts of data should be chunked
	// If in doubt, 16KB is a reasonable chunk size.
	if len(field) >= (16 * 1000) {
		return nil, nonce, ErrEncryptedFieldTooLarge
	}

	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, nonce, err
	}

	var out []byte
	encryptedField := secretbox.Seal(out[:], field, &nonce, &encryptionKey)

	return encryptedField, nonce, nil
}

// DecryptMessage uses SecretBox to decrypt the message
func DecryptMessage(encryptedField []byte, nonce []byte) (string, error) {
	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	var decryptNonce [24]byte
	copy(decryptNonce[:], nonce)
	decrypted, ok := secretbox.Open(nil, encryptedField, &decryptNonce, &encryptionKey)
	if !ok {
		return "", errors.New("Could not decrypt the value of the secret")
	}

	return string(decrypted), nil
}
