package middleware

import (
	"bytes"
	"context"
	"crypto"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/stretchr/testify/assert"
)

type mockKeystore struct {
	httpsignature.Verifier
}

func (m *mockKeystore) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	if keyID == "primary" {
		return ctx, &m.Verifier, nil
	}
	return nil, nil, nil
}

func TestHTTPSignedOnly(t *testing.T) {
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	assert.NoError(t, err)
	_, wrongKey, err := httpsignature.GenerateEd25519Key(nil)
	assert.NoError(t, err)

	keystore := mockKeystore{publicKey}

	fn1 := func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Should not have gotten here")
	}
	handler := HTTPSignedOnly(&keystore)(http.HandlerFunc(fn1))

	req, err := http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusUnauthorized, rr.Code, "request without signature should fail")

	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = "primary"
	s.Headers = []string{"digest", "(request-target)"}

	s.KeyID = "secondary"

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request with signature from wrong keyID should fail")

	s.KeyID = "primary"

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(wrongKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request with signature from wrong key should fail")

	s.Headers = []string{"digest"}

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request with signature from right key but wrong headers should fail")

	s.Headers = []string{"digest", "(request-target)"}

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	req.Body = ioutil.NopCloser(bytes.NewBuffer([]byte("hello world")))
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request with signature from right key but wrong digest should fail")

	fn2 := func(w http.ResponseWriter, r *http.Request) {
		ctxKeyID, err := GetKeyID(r.Context())
		assert.NoError(t, err, "Should be able to get key id")
		assert.Equal(t, "primary", ctxKeyID, "keyID should match")
	}
	handler = HTTPSignedOnly(&keystore)(http.HandlerFunc(fn2))

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "request with signature from right key should succeed")

	verifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)", "date"},
		},
		Keystore: &keystore,
		Opts:     crypto.Hash(0),
	}

	handler = VerifyHTTPSignedOnly(verifier)(http.HandlerFunc(fn2))

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, "request without required date should fail")

	s.Headers = []string{"digest", "(request-target)", "date"}

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", "foo")
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code, "request with invalid date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Add(time.Minute*60).Format(time.RFC1123))
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooEarly, rr.Code, "request with early date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Add(time.Minute*-60).Format(time.RFC1123))
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusRequestTimeout, rr.Code, "request with early date should fail")

	req, err = http.NewRequest("GET", "/hello-world", nil)
	assert.NoError(t, err)
	req.Header.Set("Date", time.Now().Format(time.RFC1123))
	err = s.Sign(privKey, crypto.Hash(0), req)
	assert.NoError(t, err)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code, "request with current date should succeed")
}
