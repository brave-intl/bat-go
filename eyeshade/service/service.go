package eyeshade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/datastore"
	"github.com/brave-intl/bat-go/utils/clients/common"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// Service holds info that the eyeshade router needs to operate
type Service struct {
	ctx         *context.Context
	errChannel  *chan error
	logger      *zerolog.Logger
	datastore   datastore.Datastore
	roDatastore datastore.Datastore
	clients     *common.Clients
	router      *chi.Mux
	consumers   map[string]BatchMessageConsumer
	producers   map[string]BatchMessageProducer
	dialer      *kafka.Dialer
	group       *kafka.ConsumerGroup
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

// WithContext allows you to provide the context
func WithContext(ctx context.Context) func(service *Service) error {
	return func(service *Service) error {
		service.ctx = &ctx
		return nil
	}
}

// WithContext wraps and replaces the service context
func (service *Service) WithContext(ctx context.Context) context.Context {
	nuCtx := appctx.Wrap(*service.ctx, ctx)
	service.ctx = &nuCtx
	return nuCtx
}

// WithConnections uses pre setup datastores for the service
func WithConnections(
	db datastore.Datastore,
	rodb datastore.Datastore,
) func(service *Service) error {
	return func(service *Service) error {
		service.datastore = db
		service.roDatastore = rodb
		return nil
	}
}

// Clients returns the clients needed for this service
// if not already setup, WithNewClients is run on the service
func (service *Service) Clients() *common.Clients {
	if service.clients == nil {
		err := WithNewClients(service)
		if err != nil {
			panic(fmt.Errorf("unable to setup clients, try setting up before using %v", err))
		}
	}
	return service.clients
}

// WithNewDBs sets up datastores for the service
func WithNewDBs(service *Service) error {
	eyeshadeDB, eyeshadeRODB, err := datastore.NewConnections()
	if err == nil {
		service.datastore = eyeshadeDB
		service.roDatastore = eyeshadeRODB
	}
	return err
}

// WithNewContext attaches a context to the service
func WithNewContext(service *Service) error {
	ctx := context.Background()
	service.ctx = &ctx
	return nil
}

// WithNewClients sets up a service object with the needed clients
func WithNewClients(service *Service) error {
	clients, err := common.New(
		common.WithRatios,
	)
	if err == nil {
		service.clients = clients
	}
	return err
}

// Consume has the service start consuming
func (service *Service) Consume() chan error {
	// initialize a new reader with the brokers and topic
	// the groupID identifies the consumer and prevents
	// it from receiving duplicate messages
	if service.errChannel != nil {
		return *service.errChannel
	}
	errCh := make(chan error)
	service.errChannel = &errCh
	go consume(service)
	return errCh
}

func propagateError(service *Service, err error) bool {
	logger, _ := appctx.GetLogger(service.Context())
	logger.Info().Err(err).Msg("error received from channel")
	logger.Info().
		Bool("blance-in-progress", errors.Is(err, kafka.RebalanceInProgress)).
		Bool("group-coordinator-not-available", errors.Is(err, kafka.GroupCoordinatorNotAvailable)).
		Bool("group-is-closed", errors.Is(err, kafka.ErrGroupClosed)).
		Bool("leader-not-available", errors.Is(err, kafka.LeaderNotAvailable)).
		Msg("error received")
	if err != nil &&
		!errors.Is(err, kafka.RebalanceInProgress) &&
		!errors.Is(err, kafka.GroupCoordinatorNotAvailable) &&
		!errors.Is(err, kafka.ErrGroupClosed) &&
		!errors.Is(err, kafka.LeaderNotAvailable) {
		logger.Info().Msg("unrecognized error received")
		*service.errChannel <- err
		return true
	}
	return false
}

func consume(service *Service) {
	logger, err := appctx.GetLogger(service.Context())
	if err != nil {
		*service.errChannel <- err
		return
	}
	defer func() {
		logger.Info().Err(service.CloseGroup()).Msg("group closed")
	}()
	for {
		generation, err := service.NextGroup()
		if err != nil {
			err = resolveError(service, err)
			_ = propagateError(service, err)
			continue
		}
		errCh := service.JoinGroup(generation)
		for {
			err = <-errCh
			if propagateError(service, err) {
				continue // collect more errors
			}
			keys := service.StopConsumers()
			logger.Info().
				Strs("topic-keys", keys).
				Msg("all consumers stopped")
			<-time.After(time.Second)
		}
	}
}

// StopConsumers stops consumers and waits for them to finish
func (service *Service) StopConsumers() []string {
	for _, consumer := range service.consumers {
		consumer.Stop()
	}
	keys := []string{}
	for key, consumer := range service.consumers {
		keys = append(keys, key)
		for {
			<-time.After(time.Millisecond * 100)
			if consumer.IsStopped() {
				break
			}
		}
	}
	return keys
}

// WithNewLogger attaches a logger to the context on the service
func WithNewLogger(service *Service) error {
	ctx := *service.ctx
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}
	service.ctx = &ctx
	service.logger = logger
	return nil
}

// WithBuildInfo attaches build info to context
func WithBuildInfo(service *Service) error {
	ctx := *service.ctx
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, os.Getenv("KAFKA_BROKERS"))
	ctx = context.WithValue(ctx, appctx.VersionCTXKey, os.Getenv("GIT_VERSION"))
	ctx = context.WithValue(ctx, appctx.CommitCTXKey, os.Getenv("GIT_COMMIT"))
	ctx = context.WithValue(ctx, appctx.BuildTimeCTXKey, os.Getenv("BUILD_TIME"))
	service.ctx = &ctx
	return nil
}

// Context returns the service context
func (service *Service) Context() context.Context {
	return *service.ctx
}
