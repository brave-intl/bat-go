package skus

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/cryptography"
)

func TestGenerateSecret(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldEncryptionKey := EncryptionKey
	defer func() {
		EncryptionKey = oldEncryptionKey
	}()

	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	var byteEncryptionKey [32]byte
	copy(byteEncryptionKey[:], EncryptionKey)

	s, n, err := GenerateSecret()
	if err != nil {
		t.Error("error in generate secret: ", err)
	}

	encrypted, err := hex.DecodeString(s)
	if err != nil {
		t.Error("error while decoding the encrypted string", err)
	}
	nonce, err := hex.DecodeString(n)
	if err != nil {
		t.Error("error while decoding the nonce", err)
	}

	if len(nonce) != 24 {
		t.Error("Nonce does not have correct length", err)
	}

	secretKey, err := cryptography.DecryptMessage(byteEncryptionKey, encrypted, nonce)
	if err != nil {
		t.Error("error in decrypt secret: ", err)
	}
	// secretKey is random, so i guess just make sure it is base64?
	k, err := base64.URLEncoding.DecodeString(secretKey)
	if err != nil {
		t.Error("error decoding generated secret: ", err)
	}
	if len(secretKey) != 32 {
		t.Error("Secret key does not have correct length", err)
	}
	if len(k) < 0 {
		t.Error("the key should be bigger than nothing")
	}
}

func TestSecretKey(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldEncryptionKey := EncryptionKey
	defer func() {
		EncryptionKey = oldEncryptionKey
		InitEncryptionKeys()
	}()
	EncryptionKey = "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"
	InitEncryptionKeys()

	var (
		sk, err = randomString(20)
		expiry  = time.Now().Add(1 * time.Minute)
		k       = &Key{
			ID:        "test-id",
			Name:      "test-name",
			Merchant:  "test-merchant",
			SecretKey: sk,
			CreatedAt: time.Now(),
			Expiry:    &expiry,
		}
	)

	if err != nil {
		t.Error("failed to generate a secret key: ", err)
	}
	encryptedBytes, nonceBytes, err := cryptography.EncryptMessage(byteEncryptionKey, []byte(k.SecretKey))

	k.EncryptedSecretKey = fmt.Sprintf("%x", encryptedBytes)
	k.Nonce = fmt.Sprintf("%x", nonceBytes)
	if err != nil {
		t.Error("failed to encrypt secret key: ", err)
	}

	if err := k.SetSecretKey(); err != nil {
		t.Error("failed to set secret key: ", err)
	}

	// the Secret key should now be plaintext in key, check it out
	if sk != k.SecretKey {
		t.Error("expecting initial plaintext secret key to match decrypted secret key")
	}

}
