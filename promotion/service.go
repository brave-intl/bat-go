package promotion

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/balance"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	w "github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	wallet "github.com/brave-intl/bat-go/wallet/service"
	"github.com/linkedin/goavro"
	"github.com/prometheus/client_golang/prometheus"
	kafka "github.com/segmentio/kafka-go"
	"golang.org/x/crypto/ed25519"
)

const localEnv = "local"

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
		[]string{"platform", "type", "legacy"},
	)

	// countGrantsClaimedBatTotal counts the total value of grants claimed in terms of bat ( since last start ) broken down by platform and type
	countGrantsClaimedBatTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "grants_claimed_bat_total",
			Help: "total value of grants claimed in terms of bat ( since last start ) broken down by platform and type",
		},
		[]string{"platform", "type", "legacy"},
	)
)

// SetSuggestionTopic allows for a new topic to be suggested
func SetSuggestionTopic(newTopic string) {
	suggestionTopic = newTopic
}

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
	wallet           wallet.Service
	datastore        Datastore
	roDatastore      ReadOnlyDatastore
	cbClient         cbr.Client
	reputationClient reputation.Client
	balanceClient    balance.Client
	codecs           map[string]*goavro.Codec
	kafkaWriter      *kafka.Writer
	kafkaDialer      *kafka.Dialer
	hotWallet        *uphold.Wallet
	drainChannel     chan *w.TransactionInfo
	jobs             []srv.Job
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
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
			return nil, errorutils.Wrap(err, "decrypt KAFKA_SSL_KEY failed")
		}

		keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	}

	certificate, err := tls.X509KeyPair([]byte(certPEM), keyPEM)
	if err != nil {
		return nil, errorutils.Wrap(err, "Could not parse x509 keypair")
	}

	// Define TLS configuration
	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}

	// Instrument kafka cert expiration information
	x509Cert, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, errorutils.Wrap(err, "Could not parse certificate")
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
func (s *Service) InitCodecs() error {
	s.codecs = make(map[string]*goavro.Codec)

	suggestionEventCodec, err := goavro.NewCodec(string(suggestionEventSchema))
	s.codecs["suggestion"] = suggestionEventCodec
	if err != nil {
		return err
	}
	return nil
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (s *Service) InitKafka() error {

	_, logger := logging.SetupLogger(context.Background())

	dialer, err := tlsDialer()
	if err != nil {
		return err
	}
	s.kafkaDialer = dialer

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  strings.Split(kafkaBrokers, ","),
		Topic:    suggestionTopic,
		Balancer: &kafka.LeastBytes{},
		Dialer:   dialer,
		Logger:   kafka.LoggerFunc(logger.Printf), // FIXME
	})

	s.kafkaWriter = kafkaWriter
	err = s.InitCodecs()
	if err != nil {
		return err
	}

	return nil
}

// InitHotWallet by reading the keypair and card id from the environment
func (s *Service) InitHotWallet() error {
	grantWalletPublicKeyHex := os.Getenv("GRANT_WALLET_PUBLIC_KEY")
	grantWalletPrivateKeyHex := os.Getenv("GRANT_WALLET_PRIVATE_KEY")
	grantWalletCardID := os.Getenv("GRANT_WALLET_CARD_ID")

	if len(grantWalletCardID) > 0 {
		var info w.Info
		info.Provider = "uphold"
		info.ProviderID = grantWalletCardID
		{
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		var pubKey httpsignature.Ed25519PubKey
		var privKey ed25519.PrivateKey
		var err error

		pubKey, err = hex.DecodeString(grantWalletPublicKeyHex)
		if err != nil {
			return errorutils.Wrap(err, "grantWalletPublicKeyHex is invalid")
		}
		privKey, err = hex.DecodeString(grantWalletPrivateKeyHex)
		if err != nil {
			return errorutils.Wrap(err, "grantWalletPrivateKeyHex is invalid")
		}

		s.hotWallet, err = uphold.New(info, privKey, pubKey)
		if err != nil {
			return err
		}
	} else if os.Getenv("ENV") != localEnv {
		return errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}
	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	balanceClient, err := balance.New()
	if err != nil {
		return nil, err
	}

	var reputationClient reputation.Client
	if os.Getenv("ENV") != localEnv || len(os.Getenv("REPUTATION_SERVER")) > 0 {
		reputationClient, err = reputation.New()
		if err != nil {
			return nil, err
		}
	}

	walletService, err := wallet.InitService(datastore, roDatastore)
	if err != nil {
		return nil, err
	}

	service := &Service{
		datastore:        datastore,
		roDatastore:      roDatastore,
		cbClient:         cbClient,
		reputationClient: reputationClient,
		balanceClient:    balanceClient,
		wallet:           *walletService,
	}

	// setup runnable jobs
	service.jobs = []srv.Job{
		{
			Func:    service.RunNextClaimJob,
			Cadence: 5 * time.Second,
			Workers: 1,
		},
		{
			Func:    service.RunNextSuggestionJob,
			Cadence: 5 * time.Second,
			Workers: 1,
		},
		{
			Func:    service.RunNextDrainJob,
			Cadence: 5 * time.Second,
			Workers: 1,
		},
	}

	err = service.InitKafka()
	if err != nil {
		return nil, err
	}
	err = service.InitHotWallet()
	if err != nil {
		return nil, err
	}
	return service, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (s *Service) ReadableDatastore() ReadOnlyDatastore {
	if s.roDatastore != nil {
		return s.roDatastore
	}
	return s.datastore
}

// RunNextClaimJob takes the next claim job and completes it
func (s *Service) RunNextClaimJob(ctx context.Context) (bool, error) {
	return s.datastore.RunNextClaimJob(ctx, s)
}

// RunNextSuggestionJob takes the next suggestion job and completes it
func (s *Service) RunNextSuggestionJob(ctx context.Context) (bool, error) {
	return s.datastore.RunNextSuggestionJob(ctx, s)
}

// RunNextDrainJob takes the next drain job and completes it
func (s *Service) RunNextDrainJob(ctx context.Context) (bool, error) {
	return s.datastore.RunNextDrainJob(ctx, s)
}
