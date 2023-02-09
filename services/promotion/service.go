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

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/clients/reputation"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/brave-intl/bat-go/libs/logging"
	srv "github.com/brave-intl/bat-go/libs/service"
	w "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/linkedin/goavro"
	"github.com/prometheus/client_golang/prometheus"
	kafka "github.com/segmentio/kafka-go"
	"golang.org/x/crypto/ed25519"
)

const localEnv = "local"

var (
	// toggle for drain retry job
	enableDrainRetryJob = isDrainRetryJobEnabled()
	// toggle for gemini check status
	enableGemini = isGeminiEnabled()

	suggestionTopic       = os.Getenv("ENV") + ".grant.suggestion"
	adminAttestationTopic = fmt.Sprintf("admin_attestation_events.%s.repsys.upstream", os.Getenv("ENV"))

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

func isDrainRetryJobEnabled() bool {
	var toggle = false
	if os.Getenv("DRAIN_RETRY_JOB_ENABLED") != "" {
		var err error
		toggle, err = strconv.ParseBool(os.Getenv("DRAIN_RETRY_JOB_ENABLED"))
		if err != nil {
			return false
		}
	}
	return toggle
}

func isGeminiEnabled() bool {
	var toggle = false
	if os.Getenv("GEMINI_ENABLED") != "" {
		var err error
		toggle, err = strconv.ParseBool(os.Getenv("GEMINI_ENABLED"))
		if err != nil {
			return false
		}
	}
	return toggle
}

// SetSuggestionTopic allows for a new topic to be suggested
func SetSuggestionTopic(newTopic string) {
	suggestionTopic = newTopic
}

// SetAdminAttestationTopic set admin attestation topic
func SetAdminAttestationTopic(newTopic string) {
	adminAttestationTopic = newTopic
}

// Service contains datastore and challenge bypass client connections
type Service struct {
	wallet                      *wallet.Service
	Datastore                   Datastore
	RoDatastore                 ReadOnlyDatastore
	cbClient                    cbr.Client
	reputationClient            reputation.Client
	bfClient                    bitflyer.Client
	geminiClient                gemini.Client
	geminiConf                  *gemini.Conf
	codecs                      map[string]*goavro.Codec
	kafkaWriter                 *kafka.Writer
	kafkaDialer                 *kafka.Dialer
	kafkaAdminAttestationReader kafkautils.Consumer
	hotWallet                   *uphold.Wallet
	drainChannel                chan *w.TransactionInfo
	jobs                        []srv.Job
	pauseSuggestionsUntil       time.Time
	pauseSuggestionsUntilMu     sync.RWMutex
}

// Jobs - Implement srv.JobService interface
func (service *Service) Jobs() []srv.Job {
	return service.jobs
}

// InitKafka by creating a kafka writer and creating local copies of codecs
func (service *Service) InitKafka(ctx context.Context) error {

	// TODO: eventually as cobra/viper
	ctx = context.WithValue(ctx, appctx.KafkaBrokersCTXKey, os.Getenv("KAFKA_BROKERS"))

	var err error
	service.kafkaWriter, service.kafkaDialer, err = kafkautils.InitKafkaWriter(ctx, suggestionTopic)
	if err != nil {
		return fmt.Errorf("failed to initialize kafka: %w", err)
	}

	// toggle for drain retry job
	if enableDrainRetryJob {
		groupID := os.Getenv("KAFKA_CONSUMER_GROUP_PROMOTIONS")
		if groupID == "" {
			return errors.New("failed not initialize kafka could not find consumer group")
		}

		reader, err := kafkautils.NewKafkaReader(ctx, groupID, adminAttestationTopic)
		if err != nil {
			return fmt.Errorf("failed to initialize kafka attestation reader: %w", err)
		}
		service.kafkaAdminAttestationReader = reader
	}

	service.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"suggestion":          suggestionEventSchema,
		adminAttestationTopic: adminAttestationEventSchema,
	})

	if err != nil {
		return fmt.Errorf("failed to generate codecs kafka: %w", err)
	}
	return nil
}

// InitHotWallet by reading the keypair and card id from the environment
func (service *Service) InitHotWallet(ctx context.Context) error {
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

		service.hotWallet, err = uphold.New(ctx, info, privKey, pubKey)
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
	logger := logging.Logger(ctx, "promotion.InitService")

	// register metrics with prometheus
	if err := prometheus.Register(countGrantsClaimedBatTotal); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			countGrantsClaimedBatTotal = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}
	if err := prometheus.Register(countGrantsClaimedTotal); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			countGrantsClaimedTotal = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}

	if err := prometheus.Register(countContributionsBatTotal); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			countContributionsBatTotal = ae.ExistingCollector.(*prometheus.CounterVec)
		}
	}

	if err := prometheus.Register(countContributionsTotal); err != nil {
		if ae, ok := err.(prometheus.AlreadyRegisteredError); ok {
			countContributionsTotal = ae.ExistingCollector.(*prometheus.CounterVec)
		}
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
		geminiConf   *gemini.Conf
	)
	if os.Getenv("GEMINI_ENABLED") == "true" {
		// get the correct env variables for bulk pay API call
		geminiConf = &gemini.Conf{
			ClientID: os.Getenv("GEMINI_CLIENT_ID"),
			APIKey:   os.Getenv("GEMINI_API_KEY"),
			Secret:   os.Getenv("GEMINI_API_SECRET"),
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
		{
			Func:    service.RunNextBatchPaymentsJob,
			Cadence: time.Second,
			Workers: 1,
		},
	}

	// toggle for drain  retry job
	if enableDrainRetryJob {
		service.jobs = append(service.jobs,
			srv.Job{
				Func:    service.RunNextDrainRetryJob,
				Cadence: 5 * time.Second,
				Workers: 1,
			})
	}

	// toggle for gemini check status
	if enableGemini {
		service.jobs = append(service.jobs,
			srv.Job{
				Func:    service.RunNextGeminiCheckStatus,
				Cadence: time.Second,
				Workers: 1,
			})
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
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.RoDatastore != nil {
		return service.RoDatastore
	}
	return service.Datastore
}

// RunNextMintDrainJob takes the next mint job and completes it
func (service *Service) RunNextMintDrainJob(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextMintDrainJob(ctx, service)
}

// RunNextBatchPaymentsJob takes the next claim job and completes it
func (service *Service) RunNextBatchPaymentsJob(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextBatchPaymentsJob(ctx, service)
}

// RunNextClaimJob takes the next claim job and completes it
func (service *Service) RunNextClaimJob(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextClaimJob(ctx, service)
}

// RunNextSuggestionJob takes the next suggestion job and completes it
func (service *Service) RunNextSuggestionJob(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextSuggestionJob(ctx, service)
}

// RunNextDrainJob takes the next drain job and completes it
func (service *Service) RunNextDrainJob(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextDrainJob(ctx, service)
}

// RunNextDrainRetryJob - retires failed drain jobs
func (service *Service) RunNextDrainRetryJob(ctx context.Context) (bool, error) {
	return true, service.Datastore.RunNextDrainRetryJob(ctx, service)
}

// RunNextPromotionMissingIssuer takes the next job and completes it
func (service *Service) RunNextPromotionMissingIssuer(ctx context.Context) (bool, error) {
	// get logger from context
	logger := logging.Logger(ctx, "wallet.RunNextPromotionMissingIssuer")

	// create issuer for all of the promotions without an issuer
	uuids, err := service.RoDatastore.GetPromotionsMissingIssuer(100)
	if err != nil {
		logger.Error().Err(err).Msg("failed to get promotions from datastore")
		return false, fmt.Errorf("failed to get promotions from datastore: %w", err)
	}

	for _, uuid := range uuids {
		if _, err := service.CreateIssuer(ctx, uuid, "control"); err != nil {
			logger.Error().Err(err).Msg("failed to create issuer")
		}
	}
	return true, nil
}

// RunNextGeminiCheckStatus periodically check the status of gemini claim drain transactions
func (service *Service) RunNextGeminiCheckStatus(ctx context.Context) (bool, error) {
	return service.Datastore.RunNextGeminiCheckStatus(ctx, service)
}
