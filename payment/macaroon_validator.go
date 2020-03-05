package payment

import (
	"errors"
	"strings"
	"time"
)

// CheckCaveat validates a caveat based on existing conditions
func CheckCaveat(caveat string) error {
	values := strings.Split(string(caveat), "=")
	key := strings.TrimSpace(values[0])
	value := strings.TrimSpace(values[1])

	switch key {
	case "expiry":
		{
			err := validateExpiry(value)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func validateExpiry(expiry string) error {
	expiryTime, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return err
	}

	if expiryTime.Before(time.Now()) {
		return errors.New("Token has expired")
	}
	return nil
}
