package payment

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients/cbr"
	wallet "github.com/brave-intl/bat-go/wallet/service"
)

// Service contains datastore
type Service struct {
	wallet    wallet.Service
	cbClient  cbr.Client
	datastore Datastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	walletService, err := wallet.InitService(datastore, nil)
	if err != nil {
		return nil, err
	}

	service := &Service{
		wallet:    *walletService,
		cbClient:  cbClient,
		datastore: datastore,
	}
	return service, nil
}

// RunNextOrderJob takes the next order job and completes it
func (service *Service) RunNextOrderJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextOrderJob(ctx, service)
}
