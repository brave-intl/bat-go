package payment

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/utils/cryptography"
)

// EncryptionKey for encrypting secrets
var EncryptionKey = os.Getenv("ENCRYPTION_KEY")
var byteEncryptionKey [32]byte

// What the merchant key length should be
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

// InitEncryptionKeys copies the specified encryption key into memory once
func InitEncryptionKeys() {
	copy(byteEncryptionKey[:], []byte(EncryptionKey))
}

// SetSecretKey decrypts the secret key from the database
func (key *Key) SetSecretKey() error {
	encrypted, err := hex.DecodeString(key.EncryptedSecretKey)
	if err != nil {
		return err
	}

	nonce, err := hex.DecodeString(key.Nonce)
	if err != nil {
		return err
	}

	secretKey, err := cryptography.DecryptMessage(byteEncryptionKey, encrypted, nonce)
	if err != nil {
		return err
	}

	key.SecretKey = secretKey
	return nil
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

// GenerateSecret creates a random key for merchants
func GenerateSecret() (secret string, nonce string, err error) {
	unencryptedSecret, err := randomString(keyLength)
	if err != nil {
		return "", "", err
	}
	encryptedBytes, nonceBytes, err := cryptography.EncryptMessage(byteEncryptionKey, []byte(unencryptedSecret))

	return fmt.Sprintf("%x", encryptedBytes), fmt.Sprintf("%x", nonceBytes), err
}
