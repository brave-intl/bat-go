package payments

import (
	"context"

	appsrv "github.com/brave-intl/bat-go/utils/service"
)

// Service - struct definition of payments service
type Service struct {
	baseCtx   context.Context
	secretMgr appsrv.SecretManager
}

// initialize the service
func initService(ctx context.Context) (*Service, error) {
	return &Service{
		baseCtx:   ctx,
		secretMgr: &awsClient{},
	}, nil
}
