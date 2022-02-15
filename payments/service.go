package payments

import (
	"context"

	"github.com/brave-intl/bat-go/utils/httpsignature"
)

// Service - struct definition of payments service
type Service struct{}

// LookupVerifier - implement keystore for httpsignature
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// TODO: implement
	return ctx, nil, nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context) (context.Context, *Service, error) {
	// TODO: implement
	return ctx, &Service{}, nil
}
