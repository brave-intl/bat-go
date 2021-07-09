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

func (secret TimeLimitedSecret) Derive(date time.Time, expirationDate time.Time) (string, error) {
	interval := date.Format("2006-01-02") + expirationDate.Format("2006-01-02")

	result, err := secret.hasher.HMACSha384([]byte(interval))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(result), nil
}

func (secret TimeLimitedSecret) Verify(date time.Time, expirationDate time.Time, token string) (bool, error) {
	result, err := secret.Derive(date, expirationDate)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare([]byte(result), []byte(token)) == 1, nil
}

func NewTimeLimitedSecret(secret []byte) TimeLimitedSecret {
	return TimeLimitedSecret{
		hasher: NewHMACHasher(secret),
	}

}
