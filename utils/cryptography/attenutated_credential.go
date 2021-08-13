package cryptography

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/crypto/hkdf"
)

func DecodeKeyID(keyID string) (rootKeyID string, caveats map[string]string, err error) {
	s := strings.Split(keyID, ":")
	if len(s) != 1 && len(s) != 2 {
		err = errors.New("invalid keyID")
		return
	}

	rootKeyID = s[0]
	if len(s) == 2 {
		var caveatBytes []byte
		var verifyCaveatBytes []byte
		caveatBytes, err = base64.URLEncoding.DecodeString(s[1])
		if err != nil {
			return
		}
		err = json.Unmarshal(caveatBytes, &caveats)
		if err != nil {
			return
		}

		// Verify canonical encoding by re-marshalling
		verifyCaveatBytes, err = json.Marshal(caveats)
		if err != nil {
			return
		}
		if !bytes.Equal(caveatBytes, verifyCaveatBytes) {
			err = errors.New("caveats do not match canonical encoding")
			return
		}

		return
	}
	caveats = nil
	return
}

func Attenuate(rootKeyID string, secretKey string, caveats map[string]string) (aKeyID string, aSecretKey string, err error) {
	_, c, err := DecodeKeyID(rootKeyID)
	if err != nil {
		return
	}
	if c != nil {
		err = errors.New("key has already been attenutated")
		return
	}

	if !strings.Contains(secretKey, "secret-token") {
		err = errors.New("invalid secretKey")
		return
	}

	caveatBytes, err := json.Marshal(caveats)
	if err != nil {
		return
	}
	km := hkdf.New(sha256.New, []byte(secretKey), nil, caveatBytes)

	key := make([]byte, len(secretKey))

	n, err := km.Read(key)
	if err != nil {
		return
	}
	if n != len(secretKey) {
		err = errors.New("failed to generate attenuated key")
		return
	}
	aKeyID = rootKeyID + ":" + base64.URLEncoding.EncodeToString(caveatBytes)
	aSecretKey = "secret-token:" + base64.URLEncoding.EncodeToString(key)
	return
}
