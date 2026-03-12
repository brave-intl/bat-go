package skus

// Support operator keys and middleware for the credential batch management endpoints.
//
// Requests to GET/DELETE /v1/orders/{orderID}/credentials/batches must be signed
// with an ed25519 key whose public key appears in the list for the running environment.
//
// Keys are stored in SSH authorized_keys format and are environment-specific.
// THIS FILE CONTAINS SENSITIVE INFO. Do not put it into a public repo.

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/middleware"
)

// Production and staging support operator public keys.
// TODO: replace with real operator keys before deploying to production.
var prodSupportKeys = []string{}

// Development and sandbox support operator public keys.
// TODO: add developer keys here.
var devSupportKeys = []string{}

// supportKeystore implements httpsignature.Keystore using a fixed set of ed25519 public keys.
// Each key belongs to a named support operator.
type supportKeystore struct {
	// hex-encoded public key → verifier
	keys map[string]httpsignature.Ed25519PubKey
}

func newSupportKeystore(sshPublicKeys []string) (*supportKeystore, error) {
	ks := &supportKeystore{
		keys: make(map[string]httpsignature.Ed25519PubKey, len(sshPublicKeys)),
	}

	for i, raw := range sshPublicKeys {
		pk, _, err := decodeED25519SSHKey(raw)
		if err != nil {
			return nil, fmt.Errorf("support key %d is invalid: %w", i, err)
		}

		ks.keys[hex.EncodeToString(pk)] = httpsignature.Ed25519PubKey(pk)
	}

	return ks, nil
}

func (s *supportKeystore) LookupVerifier(ctx context.Context, keyID string) (context.Context, httpsignature.Verifier, error) {
	key, ok := s.keys[keyID]
	if !ok {
		return nil, nil, errors.New("skus: unknown support operator key")
	}

	return ctx, key, nil
}

// supportKeysForEnv returns the hardcoded support operator public keys for env.
// For local/test environments, keys can be supplied via the SKUS_SUPPORT_KEYS
// environment variable as a comma-separated list of SSH authorized_keys lines.
// An error is returned for unrecognised environment names that also lack the
// env-var override, preventing silent misconfiguration.
func supportKeysForEnv(env string) ([]string, error) {
	switch env {
	case "production", "staging":
		return prodSupportKeys, nil
	case "development", "sandbox":
		return devSupportKeys, nil
	default:
		if raw := os.Getenv("SKUS_SUPPORT_KEYS"); raw != "" {
			return strings.Split(raw, ","), nil
		}

		return nil, fmt.Errorf("skus: no support keys configured for env %q and SKUS_SUPPORT_KEYS is not set", env)
	}
}

// NewSupportMwr returns middleware that requires incoming requests to carry a valid
// ed25519 HTTP signature from a known support operator key.
//
// Signed headers: date, digest, (request-target).
// The date header is checked to be within ±10 minutes of now.
func NewSupportMwr(env string) (middlewareFn, error) {
	keys, err := supportKeysForEnv(env)
	if err != nil {
		return nil, err
	}

	ks, err := newSupportKeystore(keys)
	if err != nil {
		return nil, fmt.Errorf("failed to initialise support keystore: %w", err)
	}

	verifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"date", "digest", "(request-target)"},
		},
		Keystore: ks,
		Opts:     crypto.Hash(0),
	}

	return middleware.VerifyHTTPSignedOnly(verifier), nil
}

// decodeED25519SSHKey parses an SSH authorized_keys line and returns the ed25519 public key.
func decodeED25519SSHKey(authorizedKey string) (ed25519.PublicKey, string, error) {
	parsed, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse SSH public key: %w", err)
	}

	if parsed.Type() != "ssh-ed25519" {
		return nil, "", fmt.Errorf("expected ssh-ed25519 key, got %s", parsed.Type())
	}

	cryptoKey, ok := parsed.(ssh.CryptoPublicKey)
	if !ok {
		return nil, "", fmt.Errorf("cannot extract crypto public key from SSH key")
	}

	edKey, ok := cryptoKey.CryptoPublicKey().(ed25519.PublicKey)
	if !ok {
		return nil, "", fmt.Errorf("SSH key is not an ed25519 key")
	}

	return edKey, comment, nil
}

// SignSupportRequest signs r with key using the headers expected by NewSupportMwr:
// date, digest, (request-target).
// It sets the Date header to the current time and the Digest header to the
// SHA-256 of the request body before computing the signature.
func SignSupportRequest(key ed25519.PrivateKey, r *http.Request) error {
	pubKey := key.Public().(ed25519.PublicKey)

	ps := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString(pubKey),
			Headers:   []string{"date", "digest", "(request-target)"},
		},
		Signator: key,
		Opts:     crypto.Hash(0),
	}

	r.Header.Set("Date", time.Now().UTC().Format(time.RFC1123))

	return ps.SignRequest(r)
}
