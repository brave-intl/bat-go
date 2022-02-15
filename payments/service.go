package payments

import "context"

// Service - struct definition of payments service
type Service struct{}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context) (context.Context, *Service, error) {
	// TODO: implement
	return ctx, &Service{}, nil
}
