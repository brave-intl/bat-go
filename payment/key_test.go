package payment

import (
	"testing"
	"time"
)

func TestSecretKey(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	oldAESKey := AESKey
	defer func() {
		AESKey = oldAESKey
	}()
	AESKey = "123456789012345678901234"
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
