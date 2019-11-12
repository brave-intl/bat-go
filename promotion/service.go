package promotion

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/ledger"
	"github.com/brave-intl/bat-go/utils/reputation"
	raven "github.com/getsentry/raven-go"
)

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore        Datastore
	cbClient         cbr.Client
	ledgerClient     ledger.Client
	reputationClient reputation.Client
	eventChannel     chan []byte
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

	reputationClient, err := reputation.New()
	if err != nil {
		return nil, err
	}

	return &Service{
		datastore:        datastore,
		cbClient:         cbClient,
		ledgerClient:     ledgerClient,
		reputationClient: reputationClient,
	}, nil
}

// CheckJobs starts check for unfinished jobs on a ticker
func (service *Service) CheckJobs(ctx context.Context, shouldLoop bool) {
	ticker := time.NewTicker(1000 * time.Millisecond)
	for {
		attempted, err := service.datastore.RunNextClaimJob(ctx, service)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
			break
		}
		if !attempted {
			raven.CaptureMessageAndWait("unable to attempt process claim job", nil)
			break
		}
		if !shouldLoop {
			break
		}
		<-ticker.C
	}
}
