package promotion

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/balance"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/brave-intl/bat-go/utils/logging"
	srv "github.com/brave-intl/bat-go/utils/service"
	w "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
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

	// register metrics with prometheus
	err := prometheus.Register(countGrantsClaimedBatTotal)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		countGrantsClaimedBatTotal = ae.ExistingCollector.(*prometheus.CounterVec)
	}

	err = prometheus.Register(countGrantsClaimedTotal)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		countGrantsClaimedTotal = ae.ExistingCollector.(*prometheus.CounterVec)
	}

	err = prometheus.Register(countContributionsBatTotal)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		countContributionsBatTotal = ae.ExistingCollector.(*prometheus.CounterVec)
	}

	err = prometheus.Register(countContributionsTotal)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		countContributionsTotal = ae.ExistingCollector.(*prometheus.CounterVec)
	}

	err = prometheus.Register(kafkaCertNotAfter)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		kafkaCertNotAfter = ae.ExistingCollector.(prometheus.Gauge)
	}

	err = prometheus.Register(kafkaCertNotBefore)
	if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
		kafkaCertNotBefore = ae.ExistingCollector.(prometheus.Gauge)
	}
}

// Service contains datastore and challenge bypass client connections
type Service struct {
	wallet                  *wallet.Service
	Datastore               Datastore
	RoDatastore             ReadOnlyDatastore
	cbClient                cbr.Client
	reputationClient        reputation.Client
	balanceClient           balance.Client
	codecs                  map[string]*goavro.Codec
	kafkaWriter             *kafka.Writer
	kafkaDialer             *kafka.Dialer
	hotWallet               *uphold.Wallet
	drainChannel            chan *w.TransactionInfo
	jobs                    []srv.Job
	pauseSuggestionsUntil   time.Time
	pauseSuggestionsUntilMu sync.RWMutex
}

// Jobs - Implement srv.JobService interface
func (s *Service) Jobs() []srv.Job {
	return s.jobs
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

	dialer, x509Cert, err := kafkautils.TLSDialer()
	if err != nil {
		return err
	}
	s.kafkaDialer = dialer

	kafkaCertNotBefore.Set(float64(x509Cert.NotBefore.Unix()))
	kafkaCertNotAfter.Set(float64(x509Cert.NotAfter.Unix()))

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
func (s *Service) InitHotWallet(ctx context.Context) error {
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

		s.hotWallet, err = uphold.New(ctx, info, privKey, pubKey)
		if err != nil {
			return err
		}
	} else if os.Getenv("ENV") != localEnv {
		return errors.New("GRANT_WALLET_CARD_ID must be set in production")
	}
	return nil
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(
	ctx context.Context,
	promotionDB Datastore,
	promotionRODB ReadOnlyDatastore,
	walletService *wallet.Service,
) (*Service, error) {
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

	service := &Service{
		Datastore:               promotionDB,
		RoDatastore:             promotionRODB,
		cbClient:                cbClient,
		reputationClient:        reputationClient,
		balanceClient:           balanceClient,
		wallet:                  walletService,
		pauseSuggestionsUntilMu: sync.RWMutex{},
	}

	// setup runnable jobs
	service.jobs = []srv.Job{
		{
			Func:    service.RunNextPromotionMissingIssuer,
			Cadence: 5 * time.Second,
			Workers: 1,
		},
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
	}

	var enableLinkingDraining bool
	// make sure that we only enable the DrainJob if we have linking/draining enabled
	if os.Getenv("ENABLE_LINKING_DRAINING") != "" {
		enableLinkingDraining, err = strconv.ParseBool(os.Getenv("ENABLE_LINKING_DRAINING"))
		if err != nil {
			// there was an error parsing the environment variable
			return nil, fmt.Errorf("invalid enable_linking_draining flag: %w", err)
		}
	}

	if enableLinkingDraining {
		service.jobs = append(service.jobs,
			srv.Job{
				Func:    service.RunNextDrainJob,
				Cadence: 5 * time.Second,
				Workers: 1,
			})
	}

	err = service.InitKafka()
	if err != nil {
		return nil, err
	}

	err = service.InitHotWallet(ctx)
	if err != nil {
		return nil, err
	}
	return service, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (s *Service) ReadableDatastore() ReadOnlyDatastore {
	if s.RoDatastore != nil {
		return s.RoDatastore
	}
	return s.Datastore
}

// RunNextClaimJob takes the next claim job and completes it
func (s *Service) RunNextClaimJob(ctx context.Context) (bool, error) {
	return s.Datastore.RunNextClaimJob(ctx, s)
}

// RunNextSuggestionJob takes the next suggestion job and completes it
func (s *Service) RunNextSuggestionJob(ctx context.Context) (bool, error) {
	return s.Datastore.RunNextSuggestionJob(ctx, s)
}

// RunNextDrainJob takes the next drain job and completes it
func (s *Service) RunNextDrainJob(ctx context.Context) (bool, error) {
	return s.Datastore.RunNextDrainJob(ctx, s)
}

// RunNextPromotionMissingIssuer takes the next job and completes it
func (s *Service) RunNextPromotionMissingIssuer(ctx context.Context) (bool, error) {
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	// create issuer for all of the promotions without an issuer
	uuids, err := s.RoDatastore.GetPromotionsMissingIssuer(100)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get promotions from datastore")
		return false, fmt.Errorf("failed to get promotions from datastore: %w", err)
	}

	for _, uuid := range uuids {
		if _, err := s.CreateIssuer(ctx, uuid, "control"); err != nil {
			logger.Error().Err(err).Msg("failed to create issuer")
		}
	}
	return true, nil
}
