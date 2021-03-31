package eyeshade

import (
	"bytes"
	"context"
	"net/http"

	"github.com/brave-intl/bat-go/utils/clients/common"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

// Service holds info that the eyeshade router needs to operate
type Service struct {
	datastore   Datastore
	roDatastore Datastore
	Clients     *common.Clients
	router      *chi.Mux
}

// SetupService initializes the service with the correct dependencies
func SetupService(
	options ...func(*Service) error,
) (*Service, error) {
	service := Service{}
	for _, option := range options {
		err := option(&service)
		if err != nil {
			return nil, err
		}
	}
	return &service, nil
}

// Datastore returns a read only datastore if available
// otherwise a normal datastore
func (service *Service) Datastore(ro bool) Datastore {
	if ro && service.roDatastore != nil {
		return service.roDatastore
	}
	return service.datastore
}

// StaticRouter holds static routes, not on v1 path
func (service *Service) StaticRouter() chi.Router {
	r := DefunctRouter(false)
	r.Method("GET", "/", handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handlers.Render(r.Context(), *bytes.NewBufferString("ack."), w, http.StatusOK)
	}))
	return r
}

// RouterV1 holds all of the routes under `/v1/`
func (service *Service) RouterV1() chi.Router {
	r := DefunctRouter(true)
	r.Mount("/accounts", service.AccountsRouter())
	r.Mount("/referrals", service.ReferralsRouter())
	r.Mount("/stats", service.StatsRouter())
	r.Mount("/publishers", service.SettlementsRouter())
	return r
}

// WithRouter sets up a router using the service
func WithRouter(service *Service) error {
	r := chi.NewRouter()
	r.Mount("/", service.StaticRouter())
	r.Mount("/v1/", service.RouterV1())
	service.router = r
	return nil
}

// Router returns the router that was last setup using this service
func (service *Service) Router() *chi.Mux {
	return service.router
}

// WithConnections uses pre setup datastores for the service
func WithConnections(db Datastore, rodb Datastore) func(service *Service) error {
	return func(service *Service) error {
		service.datastore = db
		service.roDatastore = rodb
		return nil
	}
}

// WithDBs sets up datastores for the service
func WithDBs(service *Service) error {
	eyeshadeDB, eyeshadeRODB, err := NewConnections()
	if err == nil {
		service.datastore = eyeshadeDB
		service.roDatastore = eyeshadeRODB
	}
	return err
}

// WithCommonClients sets up a service object with the needed clients
func WithCommonClients(service *Service) error {
	clients, err := common.New(common.Config{
		Ratios: true,
	})
	if err == nil {
		service.Clients = clients
	}
	return err
}

// AccountEarnings uses the readonly connection if available to get the account earnings
func (service *Service) AccountEarnings(
	ctx context.Context,
	options AccountEarningsOptions,
) (*[]AccountEarnings, error) {
	return service.Datastore(true).
		GetAccountEarnings(
			ctx,
			options,
		)
}

// AccountSettlementEarnings uses the readonly connection if available to get the account earnings
func (service *Service) AccountSettlementEarnings(
	ctx context.Context,
	options AccountSettlementEarningsOptions,
) (*[]AccountSettlementEarnings, error) {
	return service.Datastore(true).
		GetAccountSettlementEarnings(
			ctx,
			options,
		)
}

// Balances uses the readonly connection if available to get the account earnings
func (service *Service) Balances(
	ctx context.Context,
	accountIDs []string,
	includePending bool,
) (*[]Balance, error) {
	d := service.Datastore(true)
	balances, err := d.GetBalances(
		ctx,
		accountIDs,
	)
	if err != nil {
		return nil, err
	}
	if includePending {
		pendingVotes, err := d.GetPending(
			ctx,
			accountIDs,
		)
		if err != nil {
			return nil, err
		}
		return mergeVotes(*pendingVotes, *balances), nil
	}
	return balances, nil
}

func mergeVotes(votes []Votes, balances []Balance) *[]Balance {
	pending := []Balance{}
	balancesByAccountID := map[string]*Balance{}
	balanceIndex := map[string]int{}
	for i, balance := range balances {
		balanceIndex[balance.AccountID] = i
		balancesByAccountID[balance.AccountID] = &balance
	}
	for _, vote := range votes {
		accountID := vote.Channel
		balance := balancesByAccountID[accountID]
		if balance == nil {
			pending = append(pending, Balance{
				AccountID: accountID,
				Balance:   vote.Balance,
				Type:      "channel",
			})
		} else {
			balance := balances[balanceIndex[accountID]]
			balance.Balance = balance.Balance.Add(vote.Balance)
			balances[balanceIndex[accountID]] = balance
		}
	}
	allBalances := append(balances, pending...)
	return &allBalances
}
