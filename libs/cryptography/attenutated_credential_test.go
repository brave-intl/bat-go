package cryptography

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarshalCaveats(t *testing.T) {
	// Ensure map[string]string marshal follows canonical encoding for caveats
	expectedBytes := []byte("{\"a\":\"1\",\"b\":\"1\"}")

	// Independant of initialization order
	caveatBytes, err := json.Marshal(
		map[string]string{
			"b": "1",
			"a": "1",
		})
	assert.NoError(t, err, "marshal should not fail")
	assert.Equal(t, caveatBytes, expectedBytes, "marshaled caveats should be equal")

	caveatBytes, err = json.Marshal(
		map[string]string{
			"a": "1",
			"b": "1",
		})
	assert.NoError(t, err, "marshal should not fail")
	assert.Equal(t, caveatBytes, expectedBytes, "marshaled caveats should be equal")

	// Independant of insert order
	caveats := map[string]string{}
	caveats["b"] = "1"
	caveats["a"] = "1"
	caveatBytes, err = json.Marshal(caveats)
	assert.NoError(t, err, "marshal should not fail")
	assert.Equal(t, caveatBytes, expectedBytes, "marshaled caveats should be equal")

	caveats = map[string]string{}
	caveats["a"] = "1"
	caveats["b"] = "1"
	caveatBytes, err = json.Marshal(caveats)
	assert.NoError(t, err, "marshal should not fail")
	assert.Equal(t, caveatBytes, expectedBytes, "marshaled caveats should be equal")
}

func TestAttenuate(t *testing.T) {
	keyID := "foo"
	secretKey := "secret-token:bar"

	caveats := map[string]string{
		"merchant": "brave.com",
	}

	aKeyID, aSecretKey, err := Attenuate(keyID, secretKey, caveats)
	assert.NoError(t, err, "attenutate should succeed")

	_, _, err = Attenuate(aKeyID, aSecretKey, caveats)
	assert.Error(t, err, "must not be able to attenuate an already attenuated key")
}

func TestDecodeKeyID(t *testing.T) {
	aKeyID := "foo:eyJtZXJjaGFudCI6ImJyYXZlLmNvbSJ9"

	root, caveats, err := DecodeKeyID(aKeyID)
	assert.NoError(t, err, "decode should succeed")

	assert.Equal(t, root, "foo")
	assert.Equal(t, caveats, map[string]string{"merchant": "brave.com"})
}
