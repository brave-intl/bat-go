package promotion

import (
	"io/ioutil"
	"os"

	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/brave-intl/bat-go/utils/ledger"
	"github.com/brave-intl/bat-go/utils/reputation"
	"github.com/linkedin/goavro"
	kafka "github.com/segmentio/kafka-go"
)

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore        Datastore
	cbClient         cbr.Client
	ledgerClient     ledger.Client
	reputationClient reputation.Client
	eventChannel     chan []byte
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (service *Service) InitKafka() error {
	kafkaBrokers := os.Getenv("KAFKA_BROKERS_STRING")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  []string{kafkaBrokers},
		Topic:    "suggestion",
		Balancer: &kafka.LeastBytes{},
	})
	defer closers.Panic(kafkaWriter)

	// FIXME
	schema, err := ioutil.ReadFile("/src/schema-registry/grant/suggestion.avsc")
	if err != nil {
		return err
	}

	codec, err := goavro.NewCodec(string(schema))
	if err != nil {
		return err
	}

	service.kafkaWriter = kafkaWriter
	service.codecs = make(map[string]*goavro.Codec)
	service.codecs["suggestions"] = codec

	return nil
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

	service := &Service{
		datastore:        datastore,
		cbClient:         cbClient,
		ledgerClient:     ledgerClient,
		reputationClient: reputationClient,
	}
	err = service.InitKafka()
	if err != nil {
		return nil, err
	}
	return service, nil
}
