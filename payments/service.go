package payments

import (
	"context"

	"github.com/awslabs/amazon-qldb-driver-go/v2/qldbdriver"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
)

// Service - struct definition of payments service
type Service struct {
	// concurrent safe
	datastore *qldbdriver.QLDBDriver
}

// LookupVerifier - implement keystore for httpsignature
func (s *Service) LookupVerifier(ctx context.Context, keyID string) (context.Context, *httpsignature.Verifier, error) {
	// TODO: implement
	return ctx, nil, nil
}

// NewService creates a service using the passed datastore and clients configured from the environment
func NewService(ctx context.Context) (context.Context, *Service, error) {
	var (
		logger = logging.Logger(ctx, "payments.NewService")
	)

	driver, err := newQLDBDatastore(ctx)

	if err != nil {
		logger.Fatal().Err(err).Msg("failed to setup qldb")
	}

	return ctx, &Service{
		// initialize qldb datastore
		datastore: driver,
	}, nil
}
