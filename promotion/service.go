package promotion

import (
	"io/ioutil"
	"os"
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/ledger"
	"github.com/brave-intl/bat-go/utils/reputation"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/linkedin/goavro"
	kafka "github.com/segmentio/kafka-go"
)

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore				 Datastore
	cbClient				 cbr.Client
	ledgerClient		 ledger.Client
	reputationClient reputation.Client
	eventChannel		 chan []byte
	codec						 *goavro.Codec
	kafkaWriter			 *kafka.Writer
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

	kafkaBrokers := os.Getenv("KAFKA_BROKERS_STRING")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:	[]string{kafkaBrokers},
		Topic:		"suggestion",
		Balancer: &kafka.LeastBytes{},
	})
	defer closers.Panic(kafkaWriter)

	schema, err := ioutil.ReadFile("../../schema-registry/grant/suggestion.avsc")
	if err != nil {
		return nil, err
	}

	codec, err := goavro.NewCodec(string(schema))
	if err != nil {
		return nil, err
	}

	return &Service{
		datastore:				datastore,
		cbClient:					cbClient,
		ledgerClient:			ledgerClient,
		reputationClient: reputationClient,
		kafkaWriter:			kafkaWriter,
		codec:						codec,
	}, nil
}
