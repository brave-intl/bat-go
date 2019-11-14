package promotion

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/ledger"
	"github.com/brave-intl/bat-go/utils/reputation"
	"github.com/linkedin/goavro"
	"github.com/pkg/errors"
	kafka "github.com/segmentio/kafka-go"
)

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore        Datastore
	cbClient         cbr.Client
	ledgerClient     ledger.Client
	reputationClient reputation.Client
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
	kafkaDialer      *kafka.Dialer
}

func readFileFromEnvLoc(env string) ([]byte, error) {
	loc := os.Getenv(env)
	if len(loc) == 0 {
		return []byte{}, errors.New(env + " must be passed")
	}
	buf, err := ioutil.ReadFile(loc)
	if err != nil {
		return []byte{}, err
	}
	return buf, nil
}

func tlsDialer() (*kafka.Dialer, error) {
	keyPasswordEnv := "KAFKA_SSL_KEY_PASSWORD"
	keyPassword := os.Getenv(keyPasswordEnv)
	if len(keyPassword) == 0 {
		return nil, errors.New(keyPasswordEnv + " must be passed")
	}

	certPEM, err := readFileFromEnvLoc("KAFKA_SSL_CERTIFICATE_LOCATION")
	if err != nil {
		return nil, err
	}

	encryptedKeyPEM, err := readFileFromEnvLoc("KAFKA_SSL_KEY_LOCATION")
	if err != nil {
		return nil, err
	}

	block, rest := pem.Decode(encryptedKeyPEM)
	if len(rest) > 0 {
		return nil, errors.New("Extra data in KAFKA_SSL_KEY")
	}
	keyDER, err := x509.DecryptPEMBlock(block, []byte(keyPassword))
	if err != nil {
		return nil, errors.Wrap(err, "decrypt KAFKA_SSL_KEY failed")
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})

	// Define TLS configuration
	certificate, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return nil, err
	}

	//caCertPool := x509.NewCertPool()
	//if ok := caCertPool.AppendCertsFromPEM([]byte(caPEM)); !ok {
	//t.Error(err)
	//t.FailNow()
	//}

	return &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS: &tls.Config{
			Certificates: []tls.Certificate{certificate},
			//RootCAs:            caCertPool,
			InsecureSkipVerify: true,
		}}, nil
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (service *Service) InitKafka() error {
	dialer, err := tlsDialer()
	if err != nil {
		return err
	}
	service.kafkaDialer = dialer

	kafkaBrokers := os.Getenv("KAFKA_BROKERS_STRING")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    "suggestion",
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		//Logger:   kafka.LoggerFunc(log.Printf), // FIXME
	})

	suggestionEventCodec, err := goavro.NewCodec(string(suggestionEventSchema))
	if err != nil {
		return err
	}

	service.kafkaWriter = kafkaWriter
	service.codecs["suggestion"] = suggestionEventCodec

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
		codecs:           make(map[string]*goavro.Codec),
	}
	err = service.InitKafka()
	if err != nil {
		return nil, err
	}
	return service, nil
}
