package promotion

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/clients/balance"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/ledger"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	"github.com/linkedin/goavro"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	kafka "github.com/segmentio/kafka-go"
)

var (
	suggestionTopic = os.Getenv("ENV") + ".grant.suggestion"

	// kafkaCertNotAfter checks when the kafka certificate becomes valid
	kafkaCertNotBefore = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_before",
			Help: "Date when the kafka certificate becomes valid.",
		},
	)

	// kafkaCertNotAfter checks when the kafka certificate expires
	kafkaCertNotAfter = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "kafka_cert_not_after",
			Help: "Date when the kafka certificate expires.",
		},
	)

	// countContributionsTotal counts the number of contributions made broken down by funding and type
	countContributionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contributions_total",
			Help: "count of contributions made ( since last start ) broken down by funding and type",
		},
		[]string{"funding", "type"},
	)

	// countContributionsBatTotal counts the total value of contributions in terms of bat ( since last start ) broken down by funding and type
	countContributionsBatTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contributions_bat_total",
			Help: "total value of contributions in terms of bat ( since last start ) broken down by funding and type",
		},
		[]string{"funding", "type"},
	)

	// countGrantsClaimedTotal counts the grants claimed, broken down by platform and type
	countGrantsClaimedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grants_claimed_total",
			Help: "count of grants claimed ( since last start ) broken down by platform and type",
		},
		[]string{"platform", "type"},
	)

	// countGrantsClaimedBatTotal counts the total value of grants claimed in terms of bat ( since last start ) broken down by platform and type
	countGrantsClaimedBatTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grants_claimed_bat_total",
			Help: "total value of grants claimed in terms of bat ( since last start ) broken down by platform and type",
		},
		[]string{"platform", "type"},
	)
)

func init() {
	prometheus.MustRegister(
		countContributionsTotal,
		countContributionsBatTotal,
		countGrantsClaimedTotal,
		countGrantsClaimedBatTotal,
		kafkaCertNotBefore,
		kafkaCertNotAfter,
	)
}

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore        Datastore
	roDatastore      ReadOnlyDatastore
	cbClient         cbr.Client
	ledgerClient     ledger.Client
	reputationClient reputation.Client
	balanceClient    balance.Client
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
	kafkaDialer      *kafka.Dialer
}

func readFileFromEnvLoc(env string, required bool) ([]byte, error) {
	loc := os.Getenv(env)
	if len(loc) == 0 {
		if !required {
			return []byte{}, nil
		}
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

	caPEM, err := readFileFromEnvLoc("KAFKA_SSL_CA_LOCATION", false)
	if err != nil {
		return nil, err
	}

	certEnv := "KAFKA_SSL_CERTIFICATE"
	certPEM := []byte(os.Getenv(certEnv))
	if len(certPEM) == 0 {
		certPEM, err = readFileFromEnvLoc("KAFKA_SSL_CERTIFICATE_LOCATION", true)
		if err != nil {
			return nil, err
		}
	}

	keyEnv := "KAFKA_SSL_KEY"
	encryptedKeyPEM := []byte(os.Getenv(keyEnv))

	// Check to see if KAFKA_SSL_CERTIFICATE includes both certificate and key
	if certPEM[0] == '{' {
		type Certificate struct {
			Certificate string `json:"certificate"`
			Key         string `json:"key"`
		}
		var cert Certificate
		err := json.Unmarshal(certPEM, &cert)
		if err != nil {
			return nil, err
		}
		certPEM = []byte(cert.Certificate)
		encryptedKeyPEM = []byte(cert.Key)
	}

	if len(encryptedKeyPEM) == 0 {
		encryptedKeyPEM, err = readFileFromEnvLoc("KAFKA_SSL_KEY_LOCATION", true)
		if err != nil {
			return nil, err
		}
	}

	block, rest := pem.Decode(encryptedKeyPEM)
	if len(rest) > 0 {
		return nil, errors.New("Extra data in KAFKA_SSL_KEY")
	}

	keyPEM := pem.EncodeToMemory(block)
	if len(keyPassword) != 0 {
		keyDER, err := x509.DecryptPEMBlock(block, []byte(keyPassword))
		if err != nil {
			return nil, errors.Wrap(err, "decrypt KAFKA_SSL_KEY failed")
		}

		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	}

	certificate, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return nil, errors.Wrap(err, "Could not parse x509 keypair")
	}

	// Define TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}

	// Instrument kafka cert expiration information
	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, errors.Wrap(err, "Could not parse certificate")
	}
	kafkaCertNotBefore.Set(float64(x509Cert.NotBefore.Unix()))
	kafkaCertNotAfter.Set(float64(x509Cert.NotAfter.Unix()))

	if len(caPEM) > 0 {
		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM([]byte(caPEM)); !ok {
			return nil, errors.New("Could not add custom CA from KAFKA_SSL_CA_LOCATION")
		}
		config.RootCAs = caCertPool
	}

	return &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		TLS:       config}, nil
}

// InitCodecs used for Avro encoding / decoding
func (service *Service) InitCodecs() error {
	service.codecs = make(map[string]*goavro.Codec)

	suggestionEventCodec, err := goavro.NewCodec(string(suggestionEventSchema))
	service.codecs["suggestion"] = suggestionEventCodec
	if err != nil {
		return err
	}
	return nil
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (service *Service) InitKafka() error {
	dialer, err := tlsDialer()
	if err != nil {
		return err
	}
	service.kafkaDialer = dialer

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    suggestionTopic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(log.Printf), // FIXME
	})

	service.kafkaWriter = kafkaWriter
	err = service.InitCodecs()
	if err != nil {
		return err
	}

	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	ledgerClient, err := ledger.New()
	if err != nil {
		return nil, err
	}

	balanceClient, err := balance.New()
	if err != nil {
		return nil, err
	}

	var reputationClient *reputation.HTTPClient
	if os.Getenv("ENV") != "local" || len(os.Getenv("REPUTATION_SERVER")) > 0 {
		reputationClient, err = reputation.New()
		if err != nil {
			return nil, err
		}
	}

	service := &Service{
		datastore:        datastore,
		roDatastore:      roDatastore,
		cbClient:         cbClient,
		ledgerClient:     ledgerClient,
		reputationClient: reputationClient,
		balanceClient:    balanceClient,
	}
	err = service.InitKafka()
	if err != nil {
		return nil, err
	}
	return service, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.roDatastore != nil {
		return service.roDatastore
	}
	return service.datastore
}

// RunNextClaimJob takes the next claim job and completes it
func (service *Service) RunNextClaimJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextClaimJob(ctx, service)
}

// RunNextSuggestionJob takes the next claim job and completes it
func (service *Service) RunNextSuggestionJob(ctx context.Context) (bool, error) {
	return service.datastore.RunNextSuggestionJob(ctx, service)
}
