package payments

import (
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/go-chi/chi"
)

func TestAuthorizerSignedMiddleware(t *testing.T) {
	s := &Service{}
	executed := false
	r := chi.NewRouter()

	// setup authorization middleware
	r.Use(s.AuthorizerSignedMiddleware())

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

	// sign request
	ps := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString(pub),
			Headers: []string{
				"(request-target)",
				"host",
				"date",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: priv, // sign with priv
		Opts:     crypto.Hash(0),
	}

	// we will be signing, need all these headers for it to go through
	req.Header.Set("Host", "localhost")
	req.Header.Set("Date", time.Now().Format(time.RFC1123))
	req.Header.Set("Digest", fmt.Sprintf("%x", sha256.Sum256([]byte{})))
	req.Header.Set("Content-Length", "0")
	req.Header.Set("Content-Type", "")

	// we will be signing this header
	reqIv.Header.Set("Foo", "bar")

	// do the signature
	err = ps.SignRequest(req)
	if err != nil {
		t.Error("unexpected error signing request: ", err)
	}

	// do the signature
	err = ps.SignRequest(reqIv)
	if err != nil {
		t.Error("unexpected error signing request: ", err)
	}

	// before key is added to verifiers so like it doesnt exist
	r.ServeHTTP(wIv, reqIv)

	// add keypair to validAuthorizers
	validAuthorizers[hex.EncodeToString(pub)] = httpsignature.Ed25519PubKey(pub)
	defer func() {
		delete(validAuthorizers, hex.EncodeToString(pub))
	}()

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !executed {
		t.Error("should have executed the handler with valid signature")
	}
}
