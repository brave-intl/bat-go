package cryptography

import (
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
)

func TestEncryptionMessage(t *testing.T) {
	// set up the aes key, typically done with env variable atm
	var byteEncryptionKey [32]byte
	copy(byteEncryptionKey[:], []byte("MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0"))

	tooLarge := make([]byte, 16001)

	_, _, err := EncryptMessage(byteEncryptionKey, tooLarge)
	if !errors.Is(err, ErrEncryptedFieldTooLarge) {
		t.Error("Encrypted field failed validations", err)
	}

	encryptedBytes, n, err := EncryptMessage(byteEncryptionKey, []byte("Hello World!"))
	if err != nil {
		t.Error("error while running encrypt message", err)
	}

	encrypted, err := hex.DecodeString(fmt.Sprintf("%x", encryptedBytes))
	if err != nil {
		t.Error("error while decoding the encrypted string", err)
	}
	nonce, err := hex.DecodeString(fmt.Sprintf("%x", n))
	if err != nil {
		t.Error("error while decoding the nonce", err)
	}

	if len(nonce) != 24 {
		t.Error("Nonce does not have correct length", err)
	}

	secretKey, err := DecryptMessage(byteEncryptionKey, encrypted, nonce)
	if err != nil {
		t.Error("error in decrypt secret: ", err)
	}

	if secretKey != "Hello World!" {
		t.Error("Encryption and decryption did not work")
	}
}
