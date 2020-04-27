package crytography

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
)

// EncryptionKey for encrypting secrets
var EncryptionKey, err = base64.StdEncoding.DecodeString(os.Getenv("ENCRYPTION_KEY"))

// Both what the encryption key length should be
var keyLength = 32

// EncryptMessage uses SecretBox to encrypt the message
func EncryptMessage(message string) (secretText string, nonceString string, err error) {
	// The key argument should be 32 bytes long
	if len(EncryptionKey) != keyLength {
		return "", "", errors.New("Encryption Key is not the correct key length")
	}

	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	plaintext := []byte(message)

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", "", err
	}

	var out []byte
	ciphertext := secretbox.Seal(out[:], plaintext, &nonce, &encryptionKey)

	return fmt.Sprintf("%x", ciphertext), fmt.Sprintf("%x", nonce), nil
}

// DecryptMessage uses SecretBox to decrypt the message
func DecryptMessage(encryptedMessage string, nonceKey string) (string, error) {
	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	ciphertext, err := hex.DecodeString(encryptedMessage)
	if err != nil {
		return "", nil
	}

	nonce, err := hex.DecodeString(nonceKey)
	if err != nil {
		return "", nil
	}

	var decryptNonce [24]byte
	copy(decryptNonce[:], nonce)
	decrypted, ok := secretbox.Open(nil, ciphertext, &decryptNonce, &encryptionKey)
	if !ok {
		return "", errors.New("Could not decrypt the value of the secret")
	}

	return string(decrypted), nil
}
