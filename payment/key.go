package payment

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strings"
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
func (key *Key) SetSecretKey() {
	// FIXME call decrypt
	key.SecretKey = decryptSecretKey(key.EncryptedSecretKey, key.Nonce)
}

// Taken from https://gist.github.com/kkirsche/e28da6754c39d5e7ea10
func encryptSecretKey(secretKey string) (string, string) {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	key := []byte(AESKey)
	plaintext := []byte(secretKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}

	// Never use more than 2^32 random nonces with a given key because of the risk of a repeat.
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	return fmt.Sprintf("%x", ciphertext), fmt.Sprintf("%x", nonce)
}

func decryptSecretKey(encryptedSecretKey string, nonceKey string) string {
	// The key argument should be the AES key, either 16 or 32 bytes
	// to select AES-128 or AES-256.
	key := []byte(AESKey)
	ciphertext, _ := hex.DecodeString(encryptedSecretKey)

	nonce, _ := hex.DecodeString(nonceKey)

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		panic(err.Error())
	}

	return string(plaintext)
}

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// GenerateSecret creates a random public and secret key
func GenerateSecret() (string, string) {
	return encryptSecretKey("sk_" + randomString(keyLength))
}
