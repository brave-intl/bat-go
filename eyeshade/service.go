package eyeshade

import (
	"context"
	"os"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/utils/clients/common"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
	"github.com/rs/zerolog"
)

// Service holds info that the eyeshade router needs to operate
type Service struct {
	ctx         *context.Context
	logger      *zerolog.Logger
	datastore   Datastore
	roDatastore Datastore
	Clients     *common.Clients
	router      *chi.Mux
	consumers   map[string]BatchMessagesConsumer
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
func WithConnections(db Datastore, rodb Datastore) func(service *Service) error {
	return func(service *Service) error {
		service.datastore = db
		service.roDatastore = rodb
		return nil
	}
}

// WithNewDBs sets up datastores for the service
func WithNewDBs(service *Service) error {
	eyeshadeDB, eyeshadeRODB, err := NewConnections()
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
	clients, err := common.New(common.Config{
		Ratios: true,
	})
	if err == nil {
		service.Clients = clients
	}
	return err
}

// WithConsumer sets up a consumer on the service
func WithConsumer(
	topicHandler avro.TopicHandler,
) func(*Service) error {
	return func(service *Service) error {
		reader, config := service.NewKafkaReader(topicHandler.Topic())
		consumer := &Consumer{
			topicHandler: topicHandler,
			reader:       reader,
			config:       config,
			service:      service,
		}
		service.consumers[topicHandler.Topic()] = BatchMessagesConsumer(consumer)
		return nil
	}
}

// Consume has the service start consuming
func (service *Service) Consume() chan error {
	// initialize a new reader with the brokers and topic
	// the groupID identifies the consumer and prevents
	// it from receiving duplicate messages
	errCh := make(chan error)
	for _, consumer := range service.consumers {
		go consumer.Consume(errCh)
	}
	return errCh
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
	ctx = context.WithValue(ctx, appctx.VersionCTXKey, os.Getenv("GIT_VERSIO"))
	ctx = context.WithValue(ctx, appctx.CommitCTXKey, os.Getenv("GIT_COMMIT"))
	ctx = context.WithValue(ctx, appctx.BuildTimeCTXKey, os.Getenv("BUILD_TIME"))
	service.ctx = &ctx
	return nil
}

// Context returns the service context
func (service *Service) Context() context.Context {
	return *service.ctx
}
