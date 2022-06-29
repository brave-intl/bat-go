//go:build integration

package cbr

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"testing"

	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	_ "github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateIssuer(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	err = client.CreateIssuer(ctx, "test:"+uuid.NewV4().String(), 100)
	assert.NoError(t, err, "Should be able to create issuer")
}

func TestGetIssuer(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	issuerName := "test:" + uuid.NewV4().String()

	issuer, err := client.GetIssuer(ctx, issuerName)
	assert.Error(t, err, "Should not be able to get issuer")
	// checking the error
	httpError, ok := err.(*errorutils.ErrorBundle)
	assert.Equal(t, true, ok, "should be able to coerce to an error bundle")
	httpState, ok := httpError.Data().(clients.HTTPState)
	assert.Equal(t, true, ok, "should contain an HTTPState")
	assert.Equal(t, http.StatusNotFound, httpState.Status, "status should be not found")

	err = client.CreateIssuer(ctx, issuerName, 100)
	assert.NoError(t, err, "Should be able to create issuer")

	issuer, err = client.GetIssuer(ctx, issuerName)
	assert.NoError(t, err, "Should be able to get issuer")

	assert.NotEqual(t, len(issuer.PublicKey), 0, "Should have public key")
}

func TestSignAndRedeemCredentials(t *testing.T) {
	databaseURL := os.Getenv("CHALLENGE_BYPASS_DATABASE_URL")

	sKey := "fzJbqh6l/xWAjT6Ulmu+/Taxz8XZ7SDnJ/dUXPgtnQE="
	pKey := "jKj71sdk2XYMwZNSxvUfNkSNCUQeBuUxuTbdjIbupmE="
	blindedToken := "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	signedToken := "ohwnBITMSphAFK/06LtbC+PYl6PmmEhOdybvsfqZjG4="
	preimage := "Aa61pQzyxsy3Z6tSwccnOqiW23fNYp0z3xw6XGlA5FG8O/EqlxR87DWnas49U2JUau44dpiveAt7kBXDH5RjPQ=="
	sig := "zx1zdMhN4Et8WnrkVQOad6xhUBAJ7Pq4b8A0n96CRE0QdAQ+tJe0/eFiJqIPMuKkyfQ6VncIkGj9VzkByh9uFA=="
	payload := "test message"

	issuerName := "constant"

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		assert.NoError(t, err, "Must be able to connect to challenge-bypass db")
	}

	_, err = db.Exec("DELETE from v3_issuer_keys; DELETE FROM v3_issuers; DELETE from redemptions")
	assert.NoError(t, err, "Must be able to clear issuers")

	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	err = client.CreateIssuer(ctx, issuerName, 100)
	assert.NoError(t, err, "Should be able to create issuer")

	_, err = db.Exec("update v3_issuer_keys set signing_key=$1", sKey)
	assert.NoError(t, err, "Must be able to insert issuer key")

	issuer, err := client.GetIssuer(ctx, issuerName)
	assert.NoError(t, err, "Should be able to get issuer")
	assert.Equal(t, issuer.PublicKey, pKey, "Public key should match expected")

	resp, err := client.SignCredentials(ctx, issuerName, []string{blindedToken})
	assert.NoError(t, err, "Should be able to sign tokens")
	assert.Equal(t, resp.SignedTokens[0], signedToken, "Public key should match expected")

	err = client.RedeemCredential(ctx, issuerName, preimage, sig, payload)
	assert.NoError(t, err, "Should be able to redeem tokens")

	_, err = db.Exec("DELETE from redemptions")
	assert.NoError(t, err, "Must be able to clear redemptions")

	err = client.RedeemCredentials(ctx, []CredentialRedemption{{Issuer: issuerName, TokenPreimage: preimage, Signature: sig}}, payload)
	assert.NoError(t, err, "Should be able to bulk redeem tokens")
}
