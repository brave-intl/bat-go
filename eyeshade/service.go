package eyeshade

import (
	"bytes"
	"context"
	"net/http"

	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/utils/clients/common"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/inputs"
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
	r := RouterDefunct(false)
	r.Method("GET", "/", handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handlers.Render(r.Context(), *bytes.NewBufferString("ack."), w, http.StatusOK)
	}))
	return r
}

// RouterV1 holds all of the routes under `/v1/`
func (service *Service) RouterV1() chi.Router {
	r := RouterDefunct(true)
	r.Mount("/accounts", service.RouterAccounts())
	r.Mount("/referrals", service.RouterReferrals())
	r.Mount("/stats", service.RouterStats())
	r.Mount("/publishers", service.RouterSettlements())
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

// GetAccountEarnings uses the readonly connection if available to get the account earnings
func (service *Service) GetAccountEarnings(
	ctx context.Context,
	options AccountEarningsOptions,
) (*[]AccountEarnings, error) {
	return service.Datastore(true).
		GetAccountEarnings(
			ctx,
			options,
		)
}

// GetAccountSettlementEarnings uses the readonly connection if available to get the account earnings
func (service *Service) GetAccountSettlementEarnings(
	ctx context.Context,
	options AccountSettlementEarningsOptions,
) (*[]AccountSettlementEarnings, error) {
	return service.Datastore(true).
		GetAccountSettlementEarnings(
			ctx,
			options,
		)
}

// GetBalances uses the readonly connection if available to get the account earnings
func (service *Service) GetBalances(
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
		return mergePendingTransactions(*pendingVotes, *balances), nil
	}
	return balances, nil
}

// GetTransactions uses the readonly connection if available to get the account transactions
func (service *Service) GetTransactions(
	ctx context.Context,
	accountID string,
	txTypes []string,
) (*[]CreatorsTransaction, error) {
	transactions, err := service.Datastore(true).
		GetTransactionsByAccount(
			ctx,
			accountID,
			txTypes,
		)
	if err != nil {
		return nil, err
	}
	return transformTransactions(
		accountID,
		transactions,
	), nil
}

// GetReferralGroups gets the referral groups that match the input parameters
func (service *Service) GetReferralGroups(
	ctx context.Context,
	resolve bool,
	activeAt inputs.Time,
	fields ...string,
) (*[]countries.ReferralGroup, error) {
	groups, err := service.Datastore(true).
		GetReferralGroups(ctx, activeAt)
	if err != nil {
		return nil, err
	}
	if resolve {
		groups = countries.Resolve(*groups)
	}
	for _, group := range *groups {
		group.SetKeys(fields) // will only render these keys when serializing
	}
	return groups, nil
}

func transformTransactions(account string, txs *[]Transaction) *[]CreatorsTransaction {
	creatorsTxs := []CreatorsTransaction{}
	for _, tx := range *txs {
		creatorsTxs = append(
			creatorsTxs,
			tx.BackfillForCreators(account),
		)
	}
	return &creatorsTxs
}

func mergePendingTransactions(votes []PendingTransaction, balances []Balance) *[]Balance {
	pending := []Balance{}
	balancesByAccountID := map[string]*Balance{}
	balanceIndex := map[string]int{}
	for i, balance := range balances {
		balanceIndex[balance.AccountID] = i
		balancesByAccountID[balance.AccountID] = &balance
	}
	for _, vote := range votes {
		accountID := vote.Channel.String()
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
