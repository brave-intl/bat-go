package cryptography

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
	// incorrect time needs to be in the bucket outside (beginning of month after expirationTime)
	incorrectVerifyTime, err := time.Parse("2006-01-02", "2020-01-02")
	assert.NoError(t, err)

	correctlyVerified, err := timeLimitedSecret.Verify(metadata, correctVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.True(t, correctlyVerified, "Error verifying with correct verify time")

	// outside of the bucketed time
	incorrectlyVerified, err := timeLimitedSecret.Verify(metadata, incorrectVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.False(t, incorrectlyVerified, "Token should not have verified with incorrect verify time")

	incorrectMetadataVerified, err := timeLimitedSecret.Verify([]byte("not metadata"), correctVerifyTime, expirationTime, result)
	assert.NoError(t, err, "Error verifying token")
	assert.False(t, incorrectMetadataVerified, "Error verifying with invalid metadata")
}
