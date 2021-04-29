package promotion

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
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

// Service contains datastore and challenge bypass client connections
type Service struct {
	wallet                  *wallet.Service
	Datastore               Datastore
	RoDatastore             ReadOnlyDatastore
	cbClient                cbr.Client
	reputationClient        reputation.Client
	bfClient                bitflyer.Client
	geminiClient            gemini.Client
	geminiConf              gemini.Conf
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

// InitKafka by creating a kafka writer and creating local copies of codecs
func (s *Service) InitKafka(ctx context.Context) error {

	// TODO: eventually as cobra/viper
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, os.Getenv("KAFKA_BROKERS"))

	var err error
	s.kafkaWriter, s.kafkaDialer, err = kafkautils.InitKafkaWriter(ctx, suggestionTopic)
	if err != nil {
		return fmt.Errorf("failed to initialize kafka: %w", err)
	}

	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"suggestion": suggestionEventSchema,
	})

	if err != nil {
		return fmt.Errorf("failed to generate codecs kafka: %w", err)
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

	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		ctx, logger = logging.SetupLogger(ctx)
	}

	// register metrics with prometheus
	err = prometheus.Register(countGrantsClaimedBatTotal)
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

	cbClient, err := cbr.New()
	if err != nil {
		return nil, err
	}

	var bfClient bitflyer.Client
	// setup bfClient
	if os.Getenv("BITFLYER_ENABLED") == "true" {
		bfClient, err = bitflyer.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create bitflyer client: %w", err)
		}
		// get a fresh bf token
		// this will set the AuthToken on the client for us
		_, err = bfClient.RefreshToken(ctx, bitflyer.TokenPayloadFromCtx(ctx))
		if err != nil {
			// we don't want to stop the world if we can't connect to bf
			logger.Error().Err(err).Msg("failed to get bf access token!")
		}
	}

	var (
		geminiClient gemini.Client
		geminiConf   gemini.Conf
	)
	if os.Getenv("GEMINI_ENABLED") == "true" {
		geminiConf = gemini.Conf{
			ClientID: os.Getenv("GEMINI_CLIENT_ID"),
			APIKey:   os.Getenv("GEMINI_CLIENT_KEY"),
			Secret:   os.Getenv("GEMINI_CLIENT_SECRET"),
		}

		gc, err := gemini.New()
		if err != nil {
			return nil, fmt.Errorf("failed to create gemini client: %w", err)
		}
		geminiClient = gc
	}

	reputationClient, err := reputation.New()
	// okay to fail to make a reputation client if the environment is local
	if err != nil && os.Getenv("ENV") != localEnv {
		return nil, err
	}

	service := &Service{
		Datastore:               promotionDB,
		RoDatastore:             promotionRODB,
		cbClient:                cbClient,
		bfClient:                bfClient,
		geminiClient:            geminiClient,
		geminiConf:              geminiConf,
		reputationClient:        reputationClient,
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
		{
			Func:    service.RunNextMintDrainJob,
			Cadence: time.Second,
			Workers: 6,
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

	err = service.InitKafka(ctx)
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

// RunNextMintDrainJob takes the next mint job and completes it
func (s *Service) RunNextMintDrainJob(ctx context.Context) (bool, error) {
	return s.Datastore.RunNextMintDrainJob(ctx, s)
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
