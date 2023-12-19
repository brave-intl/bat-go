package httpsignature

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncapsulateRequest(t *testing.T) {
	var pubKey Ed25519PubKey
	pubKey, err := hex.DecodeString("e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d")
	assert.NoError(t, err)

	sig := `keyId="primary",algorithm="ed25519",headers="digest foo",signature="HvrmTu+A96H46IPZAYC2rmqRSgmgUgCcyPcnCikX0eGPSC6Va5jyr3blRLjpbGk6UMJ1FXckdWFnJxkt36gkBA=="`
	digest := "SHA-256=RK/0qy18MlBSVnWgjwz6lZEWjP/lF5HF9bvEF8FabDg="
	body := []byte("{\"hello\": \"world\"}\n")

	r, err := http.NewRequest("GET", "http://example.org/foo", ioutil.NopCloser(bytes.NewBuffer(body)))
	assert.NoError(t, err)

	r.Header.Set("Foo", "bar")
	r.Header.Set("Digest", digest)
	r.Header.Set("Signature", sig)

	// Header that does not need to be passed
	r.Header.Set("Bar", "foo")

	er, err := EncapsulateRequest(r)
	assert.NoError(t, err)

	expectedHeaders := map[string]string{
		"digest":    digest,
		"signature": sig,
		"foo":       "bar",
	}

	assert.Equal(t, body, []byte(er.Body), "Encapsulated body should be identical")
	assert.Equal(t, expectedHeaders, er.Headers, "Encapsulated headers should be identical")

	var r2 http.Request
	sp, err := er.Extract(&r2)
	assert.NoError(t, err)
	sp.Algorithm = ED25519
	sp.KeyID = "primary"
	sp.Headers = []string{"digest", "foo"}

	valid, err := sp.Verify(pubKey, crypto.Hash(0), &r2)
	assert.NoError(t, err)
	assert.Equal(t, true, valid, "The siganture should be valid after an encapsulation roundtrip")

	er.Body = "{\"world\": \"hello\"}\n"

	var r3 http.Request
	_, err = er.Extract(&r3)
	assert.NoError(t, err)

	valid, err = sp.Verify(pubKey, crypto.Hash(0), &r3)
	assert.NoError(t, err)
	assert.Equal(t, false, valid, "The siganture should be invalid since the body is different")
}

func TestEncapsulateResponse(t *testing.T) {
	var pubKey Ed25519PubKey
	pubKey, err := hex.DecodeString("e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d")
	assert.NoError(t, err)

	sig := `keyId="primary",algorithm="ed25519",headers="digest foo",signature="HvrmTu+A96H46IPZAYC2rmqRSgmgUgCcyPcnCikX0eGPSC6Va5jyr3blRLjpbGk6UMJ1FXckdWFnJxkt36gkBA=="`
	digest := "SHA-256=RK/0qy18MlBSVnWgjwz6lZEWjP/lF5HF9bvEF8FabDg="
	body := []byte("{\"hello\": \"world\"}\n")

	r := &http.Response{Header: http.Header{}}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	r.Header.Set("Foo", "bar")
	r.Header.Set("Digest", digest)
	r.Header.Set("Signature", sig)

	// Header that does not need to be passed
	r.Header.Set("Bar", "foo")

	er, err := EncapsulateResponse(context.Background(), r)
	assert.NoError(t, err)

	expectedHeaders := map[string]string{
		"digest":    digest,
		"signature": sig,
		"foo":       "bar",
	}

	assert.Equal(t, body, []byte(er.Body), "Encapsulated body should be identical")
	assert.Equal(t, expectedHeaders, er.Headers, "Encapsulated headers should be identical")

	var r2 http.Response
	sp, err := er.Extract(&r2)
	assert.NoError(t, err)
	sp.Algorithm = ED25519
	sp.KeyID = "primary"
	sp.Headers = []string{"digest", "foo"}

	valid, err := sp.VerifyResponse(pubKey, crypto.Hash(0), &r2)
	assert.NoError(t, err)
	assert.Equal(t, true, valid, "The siganture should be valid after an encapsulation roundtrip")

	er.Body = "{\"world\": \"hello\"}\n"

	var r3 http.Response
	_, err = er.Extract(&r3)
	assert.NoError(t, err)

	valid, err = sp.VerifyResponse(pubKey, crypto.Hash(0), &r3)
	assert.NoError(t, err)
	assert.Equal(t, false, valid, "The siganture should be invalid since the body is different")
}
