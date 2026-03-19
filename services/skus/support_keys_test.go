package skus

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

// genSSHKeyLine generates a fresh ed25519 keypair and returns the private key
// and a single SSH authorized_keys line for the public key.
func genSSHKeyLine(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	must.Equal(t, nil, err)

	sshPub, err := ssh.NewPublicKey(pub)
	must.Equal(t, nil, err)

	return priv, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

func TestDecodeED25519SSHKey(t *testing.T) {
	t.Run("valid_key", func(t *testing.T) {
		priv, line := genSSHKeyLine(t)

		pub, comment, err := decodeED25519SSHKey(line)
		must.Equal(t, nil, err)

		// Comment is empty when none is provided in the authorized_keys line.
		should.Equal(t, "", comment)

		// The decoded public key must match what was generated.
		should.Equal(t, ed25519.PublicKey(priv.Public().(ed25519.PublicKey)), pub)
	})

	t.Run("wrong_key_type", func(t *testing.T) {
		// A valid ECDSA key line must be rejected with a type error.
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		must.Equal(t, nil, err)

		sshPub, err := ssh.NewPublicKey(&ecKey.PublicKey)
		must.Equal(t, nil, err)

		ecLine := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
		_, _, err = decodeED25519SSHKey(ecLine)
		must.NotEqual(t, nil, err)
		should.Contains(t, err.Error(), "ssh-ed25519")
	})

	t.Run("invalid_input", func(t *testing.T) {
		_, _, err := decodeED25519SSHKey("not-a-key")
		must.NotEqual(t, nil, err)
	})
}

func TestNewSupportKeystore(t *testing.T) {
	t.Run("known_key_found", func(t *testing.T) {
		priv, line := genSSHKeyLine(t)

		ks, err := newSupportKeystore([]string{line})
		must.Equal(t, nil, err)

		pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))
		ctx, verifier, err := ks.LookupVerifier(t.Context(), pubHex)
		must.Equal(t, nil, err)
		must.NotEqual(t, nil, ctx)
		must.NotEqual(t, nil, verifier)
	})

	t.Run("unknown_key_returns_error", func(t *testing.T) {
		_, line := genSSHKeyLine(t)

		ks, err := newSupportKeystore([]string{line})
		must.Equal(t, nil, err)

		// Look up a key ID that was never registered.
		unknownHex := strings.Repeat("ab", ed25519.PublicKeySize)
		_, _, err = ks.LookupVerifier(t.Context(), unknownHex)
		must.NotEqual(t, nil, err)
		should.Contains(t, err.Error(), "unknown support operator key")
	})

	t.Run("invalid_ssh_key_rejected", func(t *testing.T) {
		_, err := newSupportKeystore([]string{"this-is-not-a-key"})
		must.NotEqual(t, nil, err)
	})

	t.Run("empty_keystore", func(t *testing.T) {
		ks, err := newSupportKeystore([]string{})
		must.Equal(t, nil, err)

		_, _, err = ks.LookupVerifier(t.Context(), "anyid")
		must.NotEqual(t, nil, err)
	})
}

func TestSupportKeysForEnv(t *testing.T) {
	t.Run("production_uses_prod_keys", func(t *testing.T) {
		keys, err := supportKeysForEnv("production")
		must.Equal(t, nil, err)
		should.Equal(t, prodSupportKeys, keys)
	})

	t.Run("staging_uses_prod_keys", func(t *testing.T) {
		keys, err := supportKeysForEnv("staging")
		must.Equal(t, nil, err)
		should.Equal(t, prodSupportKeys, keys)
	})

	t.Run("development_uses_dev_keys", func(t *testing.T) {
		keys, err := supportKeysForEnv("development")
		must.Equal(t, nil, err)
		should.Equal(t, devSupportKeys, keys)
	})

	t.Run("sandbox_uses_dev_keys", func(t *testing.T) {
		keys, err := supportKeysForEnv("sandbox")
		must.Equal(t, nil, err)
		should.Equal(t, devSupportKeys, keys)
	})

	t.Run("unknown_env_with_var_uses_var", func(t *testing.T) {
		_, line := genSSHKeyLine(t)
		t.Setenv("SKUS_SUPPORT_KEYS", line)

		keys, err := supportKeysForEnv("local")
		must.Equal(t, nil, err)
		should.Equal(t, []string{line}, keys)
	})

	t.Run("unknown_env_without_var_returns_error", func(t *testing.T) {
		t.Setenv("SKUS_SUPPORT_KEYS", "")

		_, err := supportKeysForEnv("unknown-env")
		must.NotEqual(t, nil, err)
		should.Contains(t, err.Error(), "unknown-env")
	})
}

func TestSignSupportRequest_Roundtrip(t *testing.T) {
	priv, line := genSSHKeyLine(t)

	// Inject the key via the environment variable so NewSupportMwr picks it up
	// (the test binary's ENV does not match "production"/"staging"/"development"/"sandbox",
	// so supportKeysForEnv falls through to the SKUS_SUPPORT_KEYS env var).
	t.Setenv("SKUS_SUPPORT_KEYS", line)

	mwr, err := NewSupportMwr("test")
	must.Equal(t, nil, err)

	t.Run("valid_signature_passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/v1/orders/test/credentials/batches", nil)
		must.Equal(t, nil, SignSupportRequest(priv, req))

		called := false
		handler := mwr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		should.True(t, called)
		should.Equal(t, http.StatusOK, rw.Code)
	})

	t.Run("unsigned_request_rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/v1/orders/test/credentials/batches", nil)
		// No signing.

		called := false
		handler := mwr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		should.False(t, called)
		should.NotEqual(t, http.StatusOK, rw.Code)
	})

	t.Run("wrong_key_rejected", func(t *testing.T) {
		_, wrongPriv, err := ed25519.GenerateKey(rand.Reader)
		must.Equal(t, nil, err)

		req := httptest.NewRequest(http.MethodGet, "http://localhost/v1/orders/test/credentials/batches", nil)
		must.Equal(t, nil, SignSupportRequest(wrongPriv, req))

		called := false
		handler := mwr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)

		should.False(t, called)
		should.NotEqual(t, http.StatusOK, rw.Code)
	})

}
