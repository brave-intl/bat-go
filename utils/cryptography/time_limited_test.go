package cryptography

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeLimitedCredential(t *testing.T) {
	secret := "placeholder"
	metadata := "test"
	start := time.Date(2006, time.January, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2006, time.January, 3, 0, 0, 0, 0, time.UTC)

	timeLimitedSecret := NewTimeLimitedSecret([]byte(secret))
	result, err := timeLimitedSecret.Derive([]byte(metadata), start, end)
	assert.NoError(t, err)

	if result != "/OXS+mLpp6NrDWV3dKOMBHX8lOoCnavWXvkSjPK6yye0JQwomvzoLsKqUwSB4Oya" {
		t.Error("failed to match")
	}
}

func TestTimeLimitedCredentialVerification(t *testing.T) {

	timeLimitedSecret := NewTimeLimitedSecret([]byte("1cdd2af5-1c5a-47f6-bdad-89090acee31c"))

	verifyTime, err := time.Parse("2006-01-02", "2020-12-23")
	assert.NoError(t, err)
	expirationTime, err := time.Parse("2006-01-02", "2020-12-30")
	assert.NoError(t, err)
	metadata := []byte("metadata")

	result, err := timeLimitedSecret.Derive(metadata, verifyTime, expirationTime)
	assert.NoError(t, err, "Error deriving token")

	correctVerifyTime, err := time.Parse("2006-01-02", "2020-12-23")
	assert.NoError(t, err)
	incorrectVerifyTime, err := time.Parse("2006-01-02", "2020-12-24")
	assert.NoError(t, err)

	correctlyVerified, err := timeLimitedSecret.Verify(metadata, correctVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.True(t, correctlyVerified, "Error verifying with correct verify time")

	incorrectlyVerified, err := timeLimitedSecret.Verify(metadata, incorrectVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.False(t, incorrectlyVerified, "Token should not have verified with incorrect verify time")

	incorrectMetadataVerified, err := timeLimitedSecret.Verify([]byte("not metadata"), correctVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.False(t, incorrectMetadataVerified, "Error verifying with invalid metadata")
}
