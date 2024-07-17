package payments

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
)

func TestAuthorizerSignedMiddleware(t *testing.T) {
	executed := false
	r := chi.NewRouter()

	authorizers := Authorizers{
		keys: make(map[string]httpsignature.Ed25519PubKey),
	}

	// setup authorization middleware
	r.Use(AuthorizerSignedMiddleware(&authorizers))

	r.Get("/invalid", func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not have gotten past the middleware")
		_, err := w.Write([]byte("ok"))
		if err != nil {
			t.Error("failed to write response")
		}
	})
	r.Get("/valid", func(w http.ResponseWriter, r *http.Request) {
		executed = true
		_, err := w.Write([]byte("ok"))
		if err != nil {
			t.Error("failed to write response")
		}
	})

	// no signature
	req := httptest.NewRequest("GET", "/invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// create a new keypair to test a valid signature and invalid
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Error("failed to generate key")
	}

	// bad signature before adding this keypair
	reqIv := httptest.NewRequest("GET", "/invalid", nil)
	wIv := httptest.NewRecorder()

	// good signature
	req = httptest.NewRequest("GET", "/valid", nil)

	ps := httpsignature.GetEd25519RequestSignator(priv) // sign with priv

	// we will be signing, need all these headers for it to go through
	req.Header.Set("Host", "localhost")
	req.Header.Set("Date", time.Now().Format(time.RFC1123))
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Content-Type", "")

	// we will be signing this header
	reqIv.Header.Set("Foo", "bar")

	// do the signature
	err = ps.SignRequest(req)
	assert.NoError(t, err)

	// do the signature
	err = ps.SignRequest(reqIv)
	assert.Error(t, err, "signing must fail due to absence of a header")

	reqIv.Header.Set("Host", "localhost")
	reqIv.Header.Set("Date", time.Now().Format(time.RFC1123))
	reqIv.Header.Set("Content-Length", "0")
	reqIv.Header.Set("Content-Type", "")

	err = ps.SignRequest(reqIv)
	assert.NoError(t, err)

	// before key is added to verifiers so like it doesnt exist
	r.ServeHTTP(wIv, reqIv)

	// add keypair to validAuthorizers
	authorizers.keys[hex.EncodeToString(pub)] = httpsignature.Ed25519PubKey(pub)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !executed {
		t.Error("should have executed the handler with valid signature")
	}
}
