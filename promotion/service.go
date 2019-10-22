package promotion

import (
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/ledger"
)

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore    Datastore
	cbClient     cbr.Client
	ledgerClient ledger.Client
	eventChannel chan SuggestionEvent
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}
	ledgerClient, err := ledger.New()
	if err != nil {
		return nil, err
	}

	return &Service{
		datastore:    datastore,
		cbClient:     cbClient,
		ledgerClient: ledgerClient,
	}, nil
}
