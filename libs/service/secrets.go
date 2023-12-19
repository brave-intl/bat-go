package service

import (
	"context"
)

// SecretManager - interface which allows for secret discovery, management
type SecretManager interface {
	RetrieveSecrets(context.Context, string) ([]byte, error)
}
