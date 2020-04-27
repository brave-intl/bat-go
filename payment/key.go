package payment

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/crypto/nacl/secretbox"
)

// EncryptionKey for encrypting secrets
var EncryptionKey = os.Getenv("ENCRYPTION_KEY")
var keyLength = 24

// Key represents a merchant's keys to validate skus
type Key struct {
	ID                 string     `json:"id" db:"id"`
	Name               string     `json:"name" db:"name"`
	Merchant           string     `json:"merchant" db:"merchant_id"`
	SecretKey          string     `json:"secretKey"`
	EncryptedSecretKey string     `json:"-" db:"encrypted_secret_key"`
	Nonce              string     `json:"-" db:"nonce"`
	CreatedAt          time.Time  `json:"createdAt" db:"created_at"`
	Expiry             *time.Time `json:"expiry" db:"expiry"`
}

// SetSecretKey decrypts the secret key from the database
func (key *Key) SetSecretKey() error {
	secretKey, err := decryptSecretKey(key.EncryptedSecretKey, key.Nonce)
	if err != nil {
		return err
	}

	key.SecretKey = secretKey
	return nil
}

// Taken from https://gist.github.com/kkirsche/e28da6754c39d5e7ea10
func encryptSecretKey(secretKey string) (secretText string, nonceString string, err error) {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	plaintext := []byte(secretKey)

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", "", err
	}

	var out []byte
	ciphertext := secretbox.Seal(out[:], plaintext, &nonce, &encryptionKey)

	return fmt.Sprintf("%x", ciphertext), fmt.Sprintf("%x", nonce), nil
}

func decryptSecretKey(encryptedSecretKey string, nonceKey string) (string, error) {
	var encryptionKey [32]byte
	copy(encryptionKey[:], []byte(EncryptionKey))

	ciphertext, err := hex.DecodeString(encryptedSecretKey)
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

func randomString(n int) (string, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	// Note that err == nil only if we read len(b) bytes.
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateSecret creates a random public and secret key
func GenerateSecret() (secret string, nonce string, err error) {
	unencryptedsecret, err := randomString(keyLength)
	if err != nil {
		return "", "", err
	}

	return encryptSecretKey(unencryptedsecret)
}
