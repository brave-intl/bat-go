package eyeshade

import (
	"context"

	"github.com/brave-intl/bat-go/utils/clients/common"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/go-chi/chi"
)

// Service holds info that the eyeshade router needs to operate
type Service struct {
	datastore   Datastore
	roDatastore Datastore
	Clients     *common.Clients
}

// InitService initializes the service with the correct dependencies
func InitService(
	ctx context.Context,
	datastore Datastore,
	roDatastore Datastore,
	clients *common.Clients,
) (*Service, error) {
	return &Service{
		datastore,
		roDatastore,
		clients,
	}, nil
}

// Datastore returns a read only datastore if available
// otherwise a normal datastore
func (service *Service) Datastore(ro bool) Datastore {
	if ro && service.roDatastore != nil {
		return service.roDatastore
	}
	return service.datastore
}

// SetupService generates a service and gives it to routes
func SetupService(ctx context.Context) (*chi.Mux, *Service, error) {
	r := chi.NewRouter()
	eyeshadeDB, eyeshadeRODB, err := NewConnections()
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "unable connect to eyeshade db")
	}

	clients, err := common.New(common.Config{
		Ratios: true,
	})
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "unable to generate clients")
	}

	service, err := InitService(
		ctx,
		eyeshadeDB,
		eyeshadeRODB,
		clients,
	)
	if err != nil {
		return nil, nil, errorutils.Wrap(err, "eyeshade service initialization failed")
	}

	r.Mount("/", StaticRouter())
	r.Mount("/v1/", RouterV1(service))
	return r, service, nil
}

func RouterV1(service *Service) chi.Router {
	r := DefunctRouter(true)
	r.Mount("/accounts", AccountsRouter(service))
	r.Mount("/referrals", ReferralsRouter(service))
	r.Mount("/stats", StatsRouter(service))
	r.Mount("/publishers", SettlementsRouter(service))
	return r
}

func (s *Service) AccountEarnings(
	ctx context.Context,
	options AccountEarningsOptions,
) (*[]AccountEarnings, error) {
	return s.Datastore(true).
		GetAccountEarnings(
			ctx,
			options,
		)
}
