package service

import (
	"context"

	appctx "github.com/brave-intl/bat-go/utils/context"
)

// SecretManager - interface which allows for secret discovery, management
type SecretManager interface {
	RetrieveSecrets(context.Context, string) (map[appctx.CTXKey]interface{}, error)
}
