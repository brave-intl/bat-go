package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// AESKey for encrypting secrets
var AESKey = os.Getenv("ENCRYPTION_KEY")
var keyLength = 24

// Key represents a merchant's keys to validate skus
type Key struct {
	ID                 string     `json:"id" db:"id"`
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
	key := []byte(AESKey)
	plaintext := []byte(secretKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", err
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	return fmt.Sprintf("%x", ciphertext), fmt.Sprintf("%x", nonce), nil
}

func decryptSecretKey(encryptedSecretKey string, nonceKey string) (string, error) {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	key := []byte(AESKey)
	ciphertext, _ := hex.DecodeString(encryptedSecretKey)

	nonce, _ := hex.DecodeString(nonceKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
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
