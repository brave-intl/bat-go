package grant

import (
	"context"

	srv "github.com/brave-intl/bat-go/libs/service"
	"github.com/brave-intl/bat-go/services/promotion"
	"github.com/brave-intl/bat-go/services/wallet"
)

// Service contains datastore
type Service struct {
	baseCtx     context.Context
	Datastore   Datastore
	RoDatastore ReadOnlyDatastore
	wallet      *wallet.Service
	promotion   *promotion.Service
	jobs        []srv.Job
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
}

// InitService initializes the grant service
func InitService(
	ctx context.Context,
	datastore Datastore,
	roDatastore ReadOnlyDatastore,
	walletService *wallet.Service,
	promotionService *promotion.Service,
) (*Service, error) {
	gs := &Service{
		baseCtx:     ctx,
		Datastore:   datastore,
		RoDatastore: roDatastore,
		wallet:      walletService,
		promotion:   promotionService,
	}

	// setup runnable jobs
	gs.jobs = []srv.Job{}
	return gs, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (s *Service) ReadableDatastore() ReadOnlyDatastore {
	if s.RoDatastore != nil {
		return s.RoDatastore
	}
	return s.Datastore
}
