package payment

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestGenerateSecret(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldEncryptionKey := EncryptionKey
	defer func() {
		EncryptionKey = oldEncryptionKey
	}()
	EncryptionKey = "123456789012345678901234"
	s, n, err := GenerateSecret()
	if err != nil {
		t.Error("error in generate secret: ", err)
	}
	secretKey, err := decryptSecretKey(s, n)
	if err != nil {
		t.Error("error in decrypt secret: ", err)
	}
	// secretKey is random, so i guess just make sure it is base64?
	k, err := base64.URLEncoding.DecodeString(secretKey)
	if err != nil {
		t.Error("error decoding generated secret: ", err)
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
	}()
	EncryptionKey = "123456789012345678901234"
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

	k.EncryptedSecretKey, k.Nonce, err = encryptSecretKey(k.SecretKey)
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
