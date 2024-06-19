package payments

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"testing"

	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	must "github.com/stretchr/testify/require"
)

// TestVaultSigning tests signing and validating a Vault. This test DOES NOT use Nitro PCR
// Attestation for signing and validation. It just confirms the data being signed over and
// that the signing and validation behavior works as expected.
func TestVaultSigning(t *testing.T) {
	os.Setenv("ENV", "test")

	managerKeys := vaultManagerKeys()
	threshold := 2
	shares, vaultPubkey, err := generateShares(managerKeys, threshold)
	must.Nil(t, err)
	operatorShareData, err := encryptShares(shares, managerKeys)
	must.Nil(t, err)
	vault := Vault{
		Vault: paymentLib.Vault{
			PublicKey:    vaultPubkey,
			Threshold:    threshold,
			OperatorKeys: managerKeys,
		},
		Shares: operatorShareData,
	}
	pubkey, privkey, err := ed25519.GenerateKey(rand.Reader)
	err = vault.sign(string(pubkey), privkey)
	must.Nil(t, err)
	must.True(t, ed25519.Verify(pubkey, vault.SignedData, vault.Signature))

	// Confirm that the serialized, signed values match expectations
	var signedVault Vault
	json.Unmarshal(vault.SignedData, &signedVault)
	must.Equal(t, signedVault.OperatorKeys, vault.OperatorKeys)
	must.Equal(t, signedVault.Threshold, vault.Threshold)
	must.Equal(t, signedVault.PublicKey, vault.PublicKey)
	must.Equal(t, signedVault.Shares, vault.Shares)
	must.Equal(t, len(signedVault.Signature), 0)
	must.Equal(t, signedVault.SigningPublicKey, "")
	must.Nil(t, signedVault.SignedData)
}
