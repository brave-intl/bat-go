package cryptography

import (
	"crypto/subtle"
	"encoding/base64"
	"time"
)

// TimeLimitedSecret represents a secret used to derive Time Limited Credentials
type TimeLimitedSecret struct {
	hasher HMACKey
}

// Derive - derive time limited credential based on date and expiration date
func (secret TimeLimitedSecret) Derive(metadata []byte, date time.Time, expirationDate time.Time) (string, error) {
	interval := date.Format("2006-01-02") + "/" + expirationDate.Format("2006-01-02")

	result, err := secret.hasher.HMACSha384(append(metadata, []byte(interval)...))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(result), nil
}

// Verify - verify time limited credential based on date being bound within the expiration date of the credential
func (secret TimeLimitedSecret) Verify(metadata []byte, date time.Time, expirationDate time.Time, token string) (bool, error) {
	result, err := secret.Derive(metadata, date, expirationDate)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(result), []byte(token)) == 1, nil
}

// NewTimeLimitedSecret - create a new time limited secret structure
func NewTimeLimitedSecret(secret []byte) TimeLimitedSecret {
	return TimeLimitedSecret{
		hasher: NewHMACHasher(secret),
	}
}
