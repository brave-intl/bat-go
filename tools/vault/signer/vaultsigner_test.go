//go:build integration
// +build integration

package vaultsigner

import (
	"testing"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestSign(t *testing.T) {
	wrappedClient, err := Connect()
	assert.NoError(t, err)

	key, err := httpsignature.GenerateEd25519Key()
	assert.NoError(t, err)

	name := uuid.NewV4()

	signer, err := wrappedClient.FromKey(key, "vaultsigner-test-"+name.String())
	assert.NoError(t, err)

	message := []byte("hello world")

	signature, err := signer.SignMessage(message)
	assert.NoError(t, err)

	err = key.Public().VerifySignature(message, signature)
	assert.NoError(t, err)
}

func TestVerify(t *testing.T) {
	wrappedClient, err := Connect()
	assert.NoError(t, err)

	key, err := httpsignature.GenerateEd25519Key()
	assert.NoError(t, err)
	name := uuid.NewV4()
	assert.NoError(t, err)

	signer, err := wrappedClient.FromKey(key, "vaultsigner-test-"+name.String())
	assert.NoError(t, err)

	message := []byte("hello world")

	signature, err := key.SignMessage(message)
	assert.NoError(t, err)

	err = signer.VerifySignature(message, signature)
	assert.NoError(t, err)
}
