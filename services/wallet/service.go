package wallet

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi"
	"github.com/go-chi/cors"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/backoff"
	"github.com/brave-intl/bat-go/libs/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/clients/gemini"
	"github.com/brave-intl/bat-go/libs/clients/reputation"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	srv "github.com/brave-intl/bat-go/libs/service"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/cmd"
	"github.com/brave-intl/bat-go/services/wallet/handler"
	"github.com/brave-intl/bat-go/services/wallet/metric"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/brave-intl/bat-go/services/wallet/storage"
)

// VerifiedWalletEnable enable verified wallet call
var VerifiedWalletEnable = isVerifiedWalletEnable()

func isVerifiedWalletEnable() bool {
	var toggle = false
	if os.Getenv("VERIFIED_WALLET_ENABLED") != "" {
		var err error
		toggle, err = strconv.ParseBool(os.Getenv("VERIFIED_WALLET_ENABLED"))
		if err != nil {
			return false
		}
	}
	return toggle
}

// directVerifiedWalletEnable enable direct verified wallet call
var directVerifiedWalletEnable = isDirectVerifiedWalletEnable()

func isDirectVerifiedWalletEnable() bool {
	var toggle = false
	if os.Getenv("DIRECT_VERIFIED_WALLET_ENABLED") != "" {
		var err error
		toggle, err = strconv.ParseBool(os.Getenv("DIRECT_VERIFIED_WALLET_ENABLED"))
		if err != nil {
			return false
		}
	}
	return toggle
}

var (
	// ClaimNamespace uuidv5 namespace for provider linking - exported for tests
	ClaimNamespace = uuid.Must(uuid.FromString("c39b298b-b625-42e9-a463-69c7726e5ddc"))
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusInternalServerError, http.StatusConflict}
)

var (
	errGeoCountryDisabled         = errors.New("geo country is disabled")
	errRewardsWalletAlreadyExists = errors.New("rewards wallet already exists")

	errZPInvalidIat       = errors.New("zebpay: linking info validation failed no iat")
	errZPInvalidExp       = errors.New("zebpay: linking info validation failed no exp")
	errZPInvalidAfter     = errors.New("zebpay: linking info validation failed issued at is after now")
	errZPInvalidBefore    = errors.New("zebpay: linking info validation failed expired is before now")
	errZPInvalidKYC       = errors.New("zebpay: user kyc did not pass")
	errZPInvalidDepositID = errors.New("zebpay: deposit id does not match token")
	errZPInvalidAccountID = errors.New("zebpay: account id invalid in token")

	errCustodianLinkMismatch = errors.New("wallet: custodian link mismatch")
)

// GeoValidator - interface describing validation of geolocation
type GeoValidator interface {
	Validate(ctx context.Context, geolocation string) (bool, error)
}

type challengeRepo interface {
	Get(ctx context.Context, dbi sqlx.QueryerContext, paymentID uuid.UUID) (model.Challenge, error)
	Upsert(ctx context.Context, dbi sqlx.ExecerContext, chl model.Challenge) error
	Delete(ctx context.Context, dbi sqlx.ExecerContext, paymentID uuid.UUID) error
}

type allowListRepo interface {
	GetAllowListEntry(ctx context.Context, dbi sqlx.QueryerContext, paymentID uuid.UUID) (model.AllowListEntry, error)
}

type solanaWaitlistRepo interface {
	Insert(ctx context.Context, dbi sqlx.ExecerContext, paymentID uuid.UUID, joinedAt time.Time) error
	Delete(ctx context.Context, dbi sqlx.ExecerContext, paymentID uuid.UUID) error
}

type solanaAddrsChecker interface {
	IsAllowed(ctc context.Context, addrs string) error
}

type metricSvc interface {
	LinkSuccessZP(cc string)
	LinkFailureZP(cc string)
	LinkFailureGemini(cc string)
	LinkSuccessGemini(cc string)
	LinkFailureSolanaWhitelist(cc string)
	LinkFailureSolanaRegion(cc string)
	LinkFailureSolanaChl(cc string)
	LinkFailureSolanaMsg(cc string)
	LinkSuccessSolana(cc string)
	CountDocTypeByIssuingCntry(validDocs []gemini.ValidDocument)
}

type geminiSvc interface {
	GetIssuingCountry(acc gemini.ValidatedAccount, fallback bool) string
	IsRegionAvailable(ctx context.Context, issuingCountry string, custodianRegions custodian.Regions) error
}

// Service contains datastore connections
type Service struct {
	Datastore        Datastore
	RoDatastore      ReadOnlyDatastore
	chlRepo          challengeRepo
	allowListRepo    allowListRepo
	solWaitlistRepo  solanaWaitlistRepo
	solAddrsChecker  solanaAddrsChecker
	repClient        reputation.Client
	geminiClient     gemini.Client
	geoValidator     GeoValidator
	retry            backoff.RetryFunc
	jobs             []srv.Job
	crMu             *sync.RWMutex
	custodianRegions custodian.Regions
	metric           metricSvc
	gemini           geminiSvc
	dappConf         DAppConfig
}

type DAppConfig struct {
	AllowedOrigins []string
}

// InitService creates a new instances of the wallet service.
func InitService(
	datastore Datastore,
	roDatastore ReadOnlyDatastore,
	chlRepo challengeRepo,
	allowList allowListRepo,
	solWaitlistRepo solanaWaitlistRepo,
	solAddrsChecker solanaAddrsChecker,
	repClient reputation.Client,
	geminiClient gemini.Client,
	geoCountryValidator GeoValidator,
	retry backoff.RetryFunc,
	metric metricSvc,
	gemini geminiSvc,
	dappConf DAppConfig) (*Service, error) {
	service := &Service{
		Datastore:       datastore,
		RoDatastore:     roDatastore,
		chlRepo:         chlRepo,
		allowListRepo:   allowList,
		solWaitlistRepo: solWaitlistRepo,
		solAddrsChecker: solAddrsChecker,
		repClient:       repClient,
		geminiClient:    geminiClient,
		geoValidator:    geoCountryValidator,
		retry:           retry,
		metric:          metric,
		gemini:          gemini,
		dappConf:        dappConf,
		crMu:            new(sync.RWMutex),
	}
	return service, nil
}

// Jobs - Implement srv.JobService interface
func (service *Service) Jobs() []srv.Job {
	return service.jobs
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.RoDatastore != nil {
		return service.RoDatastore
	}
	return service.Datastore
}

// SetupService - create a new wallet service
func SetupService(ctx context.Context) (context.Context, *Service) {
	l := logging.Logger(ctx, "wallet.SetupService")

	chlRepo := storage.NewChallenge()
	alRepo := storage.NewAllowList()
	solWaitlistRepo := storage.NewSolanaWaitlist()

	db, err := NewWritablePostgres(viper.GetString("datastore"), false, "wallet_db")
	if err != nil {
		l.Panic().Err(err).Msg("unable connect to wallet db")
	}

	roDB, err := NewReadOnlyPostgres(viper.GetString("ro-datastore"), false, "wallet_ro_db")
	if err != nil {
		l.Panic().Err(err).Msg("unable connect to wallet db")
	}

	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDB)
	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, db)

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))

	// jwt key is hex encoded string
	decodedBitFlyerJWTKey, err := hex.DecodeString(viper.GetString("bitflyer-jwt-key"))
	if err != nil {
		l.Error().Err(err).Msg("invalid bitflyer jwt key")
	}
	ctx = context.WithValue(ctx, appctx.BitFlyerJWTKeyCTXKey, decodedBitFlyerJWTKey)

	// setup reputation client
	repClient, err := reputation.New()
	// it's okay to not fatally fail if this environment is local, and we cant make a rep client
	if err != nil && os.Getenv("ENV") != "local" {
		l.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, repClient)

	var geminiClient gemini.Client
	if os.Getenv("GEMINI_ENABLED") == "true" {
		geminiClient, err = gemini.New()
		if err != nil {
			l.Panic().Err(err).Msg("failed to create gemini client")
		}
		ctx = context.WithValue(ctx, appctx.GeminiClientCTXKey, geminiClient)
	}

	cfg, err := appaws.BaseAWSConfig(ctx, l)
	if err != nil {
		l.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	awsClient := s3.NewFromConfig(cfg)

	// put the configured aws client on ctx
	ctx = context.WithValue(ctx, appctx.AWSClientCTXKey, awsClient)

	// get the s3 bucket and object
	bucket, bucketOK := ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	if !bucketOK {
		l.Panic().Err(errors.New("bucket not in context")).
			Msg("failed to initialize wallet service")
	}
	object, ok := ctx.Value(appctx.DisabledWalletGeoCountriesCTXKey).(string)
	if !ok {
		l.Panic().Err(errors.New("wallet geo countries disabled ctx key value not found")).
			Msg("failed to initialize wallet service")
	}

	geoCountryValidator := NewGeoCountryValidator(awsClient, Config{
		bucket: bucket,
		object: object,
	})

	mtc := metric.New()
	gemx := newGeminix("passport", "drivers_license", "national_identity_card", "passport_card")

	addrsBucket := os.Getenv("")
	if addrsBucket == "" {
		l.Panic().Err(errors.New("wallet: address bucket not found")).Msg("failed to find address bucket")
	}

	ccfg := checkerConfig{
		bucket: addrsBucket,
	}

	sac := newSolAddrsChecker(awsClient, ccfg)

	dappAO := strings.Split(os.Getenv("DAPP_ALLOWED_CORS_ORIGINS"), ",")
	if len(dappAO) == 0 {
		l.Panic().Err(errors.New("dapp allowed origins missing")).Msg("failed to initialize wallet service")
	}

	dappConf := DAppConfig{
		AllowedOrigins: dappAO,
	}

	s, err := InitService(db, roDB, chlRepo, alRepo, solWaitlistRepo, sac, repClient, geminiClient, geoCountryValidator, backoff.Retry, mtc, gemx, dappConf)
	if err != nil {
		l.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	_, err = s.RefreshCustodianRegionsWorker(ctx)
	if err != nil {
		l.Error().Err(err).Msg("failed to initialize custodian regions")
	}

	decJob := deleteExpiredChallengeTask{exec: db.RawDB(), deleter: chlRepo, deleteAfterMin: 5}

	s.jobs = []srv.Job{
		{
			Func:    s.RefreshCustodianRegionsWorker,
			Cadence: 15 * time.Minute,
			Workers: 1,
		},
		{
			Func:    decJob.deleteExpiredChallenges,
			Cadence: 10 * time.Minute,
			Workers: 1,
		},
	}

	if VerifiedWalletEnable {
		s.jobs = append(s.jobs, srv.Job{
			Func:    s.RunVerifiedWalletWorker,
			Cadence: 1 * time.Second,
			Workers: 1,
		})
	}

	err = cmd.SetupJobWorkers(ctx, s.Jobs())
	if err != nil {
		l.Error().Err(err).Msg("error initializing job workers")
	}

	return ctx, s
}

func (service *Service) SetCustodianRegions(custodianRegions custodian.Regions) {
	service.crMu.Lock()
	defer service.crMu.Unlock()
	service.custodianRegions = custodianRegions
}

func (service *Service) getCustodianRegions() custodian.Regions {
	service.crMu.RLock()
	defer service.crMu.RUnlock()
	return service.custodianRegions
}

// RegisterRoutes - register the wallet api routes given a chi.Mux
func RegisterRoutes(ctx context.Context, s *Service, r *chi.Mux, metricsMw middleware.InstrumentHandlerDef, dAppCorsMw func(next http.Handler) http.Handler, solMw func(next http.Handler) http.Handler) *chi.Mux {
	// setup our wallet routes
	r.Route("/v3/wallet", func(r chi.Router) {
		// rate limited to 2 per minute...
		// create wallet routes for our wallet providers
		r.Post("/uphold", middleware.RateLimiter(ctx, 2)(middleware.InstrumentHandlerFunc(
			"CreateUpholdWallet", CreateUpholdWalletV3)).ServeHTTP)
		r.Post("/brave", middleware.RateLimiter(ctx, 2)(middleware.InstrumentHandlerFunc(
			"CreateBraveWallet", CreateBraveWalletV3)).ServeHTTP)

		// if wallets are being migrated we do not want to over claim, we might go over the limit
		if viper.GetBool("enable-link-drain-flag") {
			// create wallet claim routes for our wallet providers
			r.Post("/uphold/{paymentID}/claim", middleware.InstrumentHandlerFunc(
				"LinkUpholdDepositAccount", LinkUpholdDepositAccountV3(s)))
			r.Post("/bitflyer/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkBitFlyerDepositAccount", LinkBitFlyerDepositAccountV3(s))).ServeHTTP)
			r.Post("/gemini/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkGeminiDepositAccount", LinkGeminiDepositAccountV3(s))).ServeHTTP)
			r.Post("/zebpay/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkZebPayDepositAccount", LinkZebPayDepositAccountV3(s))).ServeHTTP)

			// create wallet connect routes for our wallet providers
			r.Post("/uphold/{paymentID}/connect", middleware.InstrumentHandlerFunc(
				"LinkUpholdDepositAccount", LinkUpholdDepositAccountV3(s)))
			r.Post("/bitflyer/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkBitFlyerDepositAccount", LinkBitFlyerDepositAccountV3(s))).ServeHTTP)
			r.Post("/gemini/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkGeminiDepositAccount", LinkGeminiDepositAccountV3(s))).ServeHTTP)
			r.Post("/zebpay/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkZebPayDepositAccount", LinkZebPayDepositAccountV3(s))).ServeHTTP)

			r.Method(http.MethodPost, "/solana/{paymentID}/connect", metricsMw("LinkSolanaAddress", dAppCorsMw(LinkSolanaAddress(s))))
			r.Method(http.MethodOptions, "/solana/{paymentID}/connect", metricsMw("LinkSolanaAddressOptions", dAppCorsMw(noOpHandler())))
		}

		r.Get("/linking-info", middleware.SimpleTokenAuthorizedOnly(middleware.InstrumentHandlerFunc("GetLinkingInfo", GetLinkingInfoV3(s))).ServeHTTP)

		// get wallet routes
		r.Get("/{paymentID}", middleware.InstrumentHandlerFunc("GetWallet", GetWalletV3))
		r.Get("/recover/{publicKey}", middleware.InstrumentHandlerFunc("RecoverWallet", RecoverWalletV3))

		// get wallet balance routes
		r.Get("/uphold/{paymentID}", middleware.InstrumentHandlerFunc("GetUpholdWalletBalance", GetUpholdWalletBalanceV3))

		r.Post("/challenges", middleware.RateLimiter(ctx, 2)(metricsMw("CreateChallenge", dAppCorsMw(CreateChallenge(s)))).ServeHTTP)
		r.Options("/challenges", middleware.RateLimiter(ctx, 2)(metricsMw("CreateChallengeOptions", dAppCorsMw(noOpHandler()))).ServeHTTP)

		{
			solH := handler.NewSolana(s)

			r.Post("/solana/waitlist", middleware.HTTPSignedOnly(s)(metricsMw("SolanaPostWaitlist", handlers.AppHandler(solH.PostWaitlist))).ServeHTTP)

			r.Delete("/solana/waitlist/{paymentID}", middleware.HTTPSignedOnly(s)(metricsMw("SolanaDeleteWaitlist", handlers.AppHandler(solH.DeleteWaitlist))).ServeHTTP)

			r.Options("/solana/waitlist", middleware.RateLimiter(ctx, 5)(metricsMw("SolanaWaitlistOptions", solMw(noOpHandler()))).ServeHTTP)
		}
	})

	r.Route("/v4/wallets", func(r chi.Router) {
		r.Post("/", middleware.RateLimiter(ctx, 2)(
			middleware.InstrumentHandlerFunc("CreateWalletV4", CreateWalletV4(s))).ServeHTTP)

		r.Patch("/{paymentID}", middleware.RateLimiter(ctx, 2)(middleware.HTTPSignedOnly(s)(
			middleware.InstrumentHandlerFunc("UpdateWalletV4", UpdateWalletV4(s)))).ServeHTTP)

		r.Get("/{paymentID}", middleware.RateLimiter(ctx, 7)(middleware.HTTPSignedOnly(s)(
			middleware.InstrumentHandlerFunc("GetWalletV4", GetWalletV4(s)))).ServeHTTP)

		r.Get("/uphold/{paymentID}", middleware.RateLimiter(ctx, 2)(middleware.HTTPSignedOnly(s)(
			middleware.InstrumentHandlerFunc("GetUpholdWalletBalanceV4", GetUpholdWalletBalanceV4))).ServeHTTP)
	})

	return r
}

// TODO(clD11): WR. Move once we address the rest_run.go and grant.go start functions.

func NewDAppCorsMw(origins []string) func(next http.Handler) http.Handler {
	opts := cors.Options{
		Debug:            false,
		AllowedOrigins:   origins,
		AllowedHeaders:   []string{"Accept", "Content-Type"},
		ExposedHeaders:   []string{""},
		AllowedMethods:   []string{http.MethodPost},
		AllowCredentials: false,
		MaxAge:           300,
	}
	return cors.Handler(opts)
}

func noOpHandler() http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		return
	})
}

func NewCORSMwr(opts cors.Options, methods ...string) func(next http.Handler) http.Handler {
	opts.AllowedMethods = methods

	return cors.Handler(opts)
}

func NewCORSOpts(origins []string, dbg bool) cors.Options {
	result := cors.Options{
		Debug:            dbg,
		AllowedOrigins:   origins,
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{""},
		AllowCredentials: false,
		MaxAge:           300, // Maximum value not ignored by any of major browsers
	}

	return result
}

// SubmitAnonCardTransaction validates and submits a transaction on behalf of an anonymous card
func (service *Service) SubmitAnonCardTransaction(
	ctx context.Context,
	walletID uuid.UUID,
	transaction string,
	destination string,
) (*walletutils.TransactionInfo, error) {
	info, err := service.Datastore.GetWallet(ctx, walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}
	return service.SubmitCommitableAnonCardTransaction(ctx, info, transaction, destination, true)
}

// GetWallet - get a wallet by id
func (service *Service) GetWallet(ctx context.Context, ID uuid.UUID) (*walletutils.Info, error) {
	return service.Datastore.GetWallet(ctx, ID)
}

// SubmitCommitableAnonCardTransaction submits a transaction
func (service *Service) SubmitCommitableAnonCardTransaction(
	ctx context.Context,
	info *walletutils.Info,
	transaction string,
	destination string,
	confirm bool,
) (*walletutils.TransactionInfo, error) {
	providerWallet, err := provider.GetWallet(ctx, *info)
	if err != nil {
		return nil, err
	}
	anonCard, ok := providerWallet.(*uphold.Wallet)
	if !ok {
		return nil, errors.New("only uphold wallets are supported")
	}

	// FIXME needs to require the idempotency key
	_, err = anonCard.VerifyAnonCardTransaction(ctx, transaction, destination)
	if err != nil {
		return nil, err
	}

	// Submit and confirm since we are requiring the idempotency key
	return anonCard.SubmitTransaction(ctx, transaction, confirm)
}

// GetLinkingInfo - Get data about the linking info
func (service *Service) GetLinkingInfo(ctx context.Context, providerLinkingID, custodianID string) (map[string]LinkingInfo, error) {
	// compute the provider linking id based on custodian id if there is one

	if custodianID != "" {
		// generate a provider linking id
		providerLinkingID = uuid.NewV5(ClaimNamespace, custodianID).String()
	}

	infos, err := service.Datastore.GetLinkingLimitInfo(ctx, providerLinkingID)
	if err != nil {
		return infos, fmt.Errorf("unable to increase linking limit: %w", err)
	}
	return infos, nil
}

// LinkBitFlyerWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkBitFlyerWallet(ctx context.Context, walletID uuid.UUID, depositID, accountHash string) (string, error) {
	const (
		depositProvider = "bitflyer"
		country         = "JP"
	)

	// In the controller validation, we verified that the account hash and deposit id were signed by bitflyer
	// we also validated that this "info" signed the request to perform the linking with http signature
	// we assume that since we got linkingInfo signed from BF that they are KYC
	providerLinkingID := uuid.NewV5(ClaimNamespace, accountHash)
	if err := service.linkCustodialAccount(ctx, walletID.String(), depositID, providerLinkingID, depositProvider, country); err != nil {
		if errors.Is(err, ErrUnusualActivity) {
			return "", handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}

		if errors.Is(err, ErrGeoResetDifferent) {
			return "", handlers.WrapError(err, "mismatched provider account regions", http.StatusBadRequest)
		}

		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}

		return "", handlers.WrapError(err, "unable to link bitflyer wallets", status)
	}

	return country, nil
}

// LinkZebPayWallet links a wallet and transfers funds to newly linked wallet.
func (service *Service) LinkZebPayWallet(ctx context.Context, walletID uuid.UUID, verificationToken string) (string, error) {
	const depositProvider = "zebpay"

	claims, err := parseZebPayClaims(ctx, verificationToken)
	if err != nil {
		return "", err
	}

	if err := claims.validate(time.Now()); err != nil {
		service.metric.LinkFailureZP(claims.CountryCode)
		return "", err
	}

	providerLinkingID := uuid.NewV5(ClaimNamespace, claims.AccountID)
	if err := service.linkCustodialAccount(ctx, walletID.String(), claims.DepositID, providerLinkingID, depositProvider, claims.CountryCode); err != nil {
		service.metric.LinkFailureZP(claims.CountryCode)

		if errors.Is(err, ErrUnusualActivity) {
			return "", handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}

		if errors.Is(err, ErrGeoResetDifferent) {
			return "", handlers.WrapError(err, "mismatched provider account regions", http.StatusBadRequest)
		}

		if errors.Is(err, ErrTooManyCardsLinked) {
			return "", handlers.WrapError(err, "unable to link zebpay wallets", http.StatusConflict)
		}

		return "", handlers.WrapError(err, "unable to link zebpay wallets", http.StatusInternalServerError)
	}

	service.metric.LinkSuccessZP(claims.CountryCode)

	return claims.CountryCode, nil
}

const errNoAcceptedDocumentType model.Error = "no accepted document type"

// LinkGeminiWallet links a wallet to a Gemini account.
func (service *Service) LinkGeminiWallet(ctx context.Context, walletID uuid.UUID, verificationToken, depositID string) (string, error) {
	cl, err := service.Datastore.GetCustodianLinkByWalletID(ctx, walletID)
	if err != nil && !errors.Is(err, model.ErrNoWalletCustodian) {
		return "", handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
	}

	const depositProvider = "gemini"

	if cl.isLinked() && !strings.EqualFold(cl.Custodian, depositProvider) {
		return "", errCustodianLinkMismatch
	}

	gc, ok := ctx.Value(appctx.GeminiClientCTXKey).(gemini.Client)
	if !ok {
		return "", handlers.WrapError(appctx.ErrNotInContext, "gemini client misconfigured", http.StatusInternalServerError)
	}

	acc, err := gc.FetchValidatedAccount(ctx, verificationToken, depositID)
	if err != nil {
		return "", fmt.Errorf("failed to validate account: %w", err)
	}

	service.metric.CountDocTypeByIssuingCntry(acc.ValidDocuments)

	linkingID := uuid.NewV5(ClaimNamespace, acc.ID)
	// Some Gemini accounts do not have valid documents setup. For accounts that are already linked i.e. are
	// re-authenticating we can fall back to the legacy country code. New or re-linkings should not fall back.
	isAuth := cl.isLinked() && *cl.LinkingID == linkingID
	issuingCountry := service.gemini.GetIssuingCountry(acc, isAuth)
	if issuingCountry == "" {
		return "", fmt.Errorf("failed to validate account: %w", errNoAcceptedDocumentType)
	}

	if err := service.gemini.IsRegionAvailable(ctx, issuingCountry, service.custodianRegions); err != nil {
		if errors.Is(err, errorutils.ErrInvalidCountry) {
			// If a wallet has previously been linked i.e. has a prior linking, but the country is now invalid/blocked
			// then we can allow the account to link due to its prior successful linking i.e. it is grandfathered.
			// If there is no prior linking and the country is invalid/blocked then we should apply the current rules and block it.
			hasPriorLinking, priorLinkingErr := service.Datastore.HasPriorLinking(ctx, walletID, linkingID)
			if priorLinkingErr != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("failed to check prior linkings: %w", priorLinkingErr)
			}

			if !hasPriorLinking {
				service.metric.LinkFailureGemini(issuingCountry)
				return "", fmt.Errorf("failed to validate account: %w", err)
			}

		} else {
			// not err invalid country error
			return "", fmt.Errorf("failed to validate account: %w", err)
		}
	}
	service.metric.LinkSuccessGemini(issuingCountry)

	if err := service.linkCustodialAccount(ctx, walletID.String(), depositID, linkingID, depositProvider, issuingCountry); err != nil {
		if errors.Is(err, ErrUnusualActivity) {
			return "", handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}

		if errors.Is(err, ErrGeoResetDifferent) {
			return "", handlers.WrapError(err, "mismatched provider account regions", http.StatusBadRequest)
		}

		if errors.Is(err, ErrTooManyCardsLinked) {
			return "", handlers.WrapError(err, "unable to link gemini wallets", http.StatusConflict)
		}

		return "", handlers.WrapError(err, "unable to link gemini wallets", http.StatusInternalServerError)
	}

	return issuingCountry, nil
}

// LinkUpholdWallet links an uphold.Wallet and transfers funds.
func (service *Service) LinkUpholdWallet(ctx context.Context, wallet uphold.Wallet, transaction string, _ *uuid.UUID) (string, error) {
	const depositProvider = "uphold"
	// do not confirm this transaction yet
	info := wallet.GetWalletInfo()

	var (
		userID  string
		country string
		probi   decimal.Decimal
	)

	transactionInfo, err := wallet.VerifyTransaction(ctx, transaction)
	if err != nil {
		return "", handlers.WrapError(
			errors.New("failed to verify transaction"), "transaction verification failure",
			http.StatusForbidden)
	}

	// add custodian regions to ctx going to client
	_, ok := ctx.Value(appctx.CustodianRegionsCTXKey).(custodian.Regions)
	if !ok {
		cr := service.getCustodianRegions()
		ctx = context.WithValue(ctx, appctx.CustodianRegionsCTXKey, &cr)
	}

	walletID, err := uuid.FromString(info.ID)
	if err != nil {
		return "", fmt.Errorf("failed to parse uphold id: %w", err)
	}

	// verify that the user is kyc from uphold. (for all wallet provider cases)
	if uID, ok, c, err := wallet.IsUserKYC(ctx, transactionInfo.Destination); err != nil {
		// check if this gemini accountID has already been linked to this wallet,
		if errors.Is(err, errorutils.ErrInvalidCountry) {
			ok, priorLinkingErr := service.Datastore.HasPriorLinking(
				ctx, walletID, uuid.NewV5(ClaimNamespace, userID))
			if priorLinkingErr != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("failed to check prior linkings: %w", priorLinkingErr)
			}
			// if a wallet has a prior linking to this account, allow the invalid country, otherwise
			// return the kyc error
			if !ok {
				// then pass back the original geo error
				return "", err
			}
			// allow invalid country if there was a prior linking
		} else {
			return "", fmt.Errorf("wallet could not be kyc checked: %w", err)
		}
	} else if !ok {
		// fail
		return "", handlers.WrapError(
			errors.New("user kyc did not pass"),
			"KYC required",
			http.StatusForbidden)
	} else {
		userID = uID
		country = c
	}

	// check kyc user id validity
	if userID == "" {
		return "", handlers.WrapError(
			errors.New("user id not provided"),
			"KYC required",
			http.StatusForbidden)
	}

	probi = transactionInfo.Probi

	providerLinkingID := uuid.NewV5(ClaimNamespace, userID)
	if err := service.linkCustodialAccount(ctx, walletID.String(), transactionInfo.Destination, providerLinkingID, depositProvider, country); err != nil {
		if errors.Is(err, ErrUnusualActivity) {
			return "", handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}

		if errors.Is(err, ErrGeoResetDifferent) {
			return "", handlers.WrapError(err, "mismatched provider account regions", http.StatusBadRequest)
		}

		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}

		return "", handlers.WrapError(err, "unable to link uphold wallets", status)
	}

	// if this wallet is linking a deposit account do not submit a transaction
	if decimal.NewFromFloat(0).LessThan(probi) {
		_, err := service.SubmitCommitableAnonCardTransaction(ctx, &info, transaction, "", true)
		if err != nil {
			return "", handlers.WrapError(err, "unable to transfer tokens", http.StatusBadRequest)
		}
	}

	return country, nil
}

const errDisabledRegion model.Error = "disabled region"

func (service *Service) LinkSolanaAddress(ctx context.Context, paymentID uuid.UUID, req linkSolanaAddrRequest) error {
	if err := service.solAddrsChecker.IsAllowed(ctx, req.SolanaPublicKey); err != nil {
		return err
	}

	repSum, err := service.repClient.GetReputationSummary(ctx, paymentID)
	if err != nil {
		return err
	}

	if !service.custodianRegions.Solana.Verdict(repSum.GeoCountry) {
		service.metric.LinkFailureSolanaRegion(repSum.GeoCountry)
		return errDisabledRegion
	}

	if err := isWalletWhitelisted(ctx, service.Datastore.RawDB(), service.allowListRepo, paymentID); err != nil {
		service.metric.LinkFailureSolanaWhitelist(repSum.GeoCountry)
		return err
	}

	ctx, txn, rollback, commit, err := getTx(ctx, service.Datastore)
	if err != nil {
		return err
	}
	defer rollback()

	chl, err := service.chlRepo.Get(ctx, txn, paymentID)
	if err != nil {
		return err
	}

	if err := chl.IsValid(time.Now()); err != nil {
		service.metric.LinkFailureSolanaChl(repSum.GeoCountry)
		return err
	}

	w, err := service.Datastore.GetWallet(ctx, paymentID)
	if err != nil {
		return err
	}

	if w == nil {
		return model.ErrWalletNotFound
	}

	if err := w.LinkSolanaAddress(ctx, walletutils.SolanaLinkReq{
		Pub:   req.SolanaPublicKey,
		Sig:   req.SolanaSignature,
		Msg:   req.Message,
		Nonce: chl.Nonce,
	}); err != nil {
		service.metric.LinkFailureSolanaMsg(repSum.GeoCountry)
		return err
	}

	cl := NewSolanaCustodialLink(paymentID, w.UserDepositDestination)

	if err := service.Datastore.LinkWallet(ctx, w.ID, w.UserDepositDestination, *cl.LinkingID, cl.Custodian); err != nil {
		return err
	}

	if err := service.chlRepo.Delete(ctx, txn, chl.PaymentID); err != nil {
		return err
	}

	if err := commit(); err != nil {
		return err
	}

	service.metric.LinkSuccessSolana(repSum.GeoCountry)

	return nil
}

func (service *Service) linkCustodialAccount(ctx context.Context, wID string, userDepositDestination string, providerLinkingID uuid.UUID, depositProvider, country string) error {
	walletID, err := uuid.FromString(wID)
	if err != nil {
		return fmt.Errorf("invalid wallet id, not uuid: %w", err)
	}

	repClient, ok := ctx.Value(appctx.ReputationClientCTXKey).(reputation.Client)
	if !ok {
		return ErrNoReputationClient
	}

	if _, _, err := repClient.IsLinkingReputable(ctx, walletID, country); err != nil {
		return fmt.Errorf("failed to check wallet rep: %w", err)
	}

	return service.Datastore.LinkWallet(ctx, walletID.String(), userDepositDestination, providerLinkingID, depositProvider)
}

func (service *Service) CreateChallenge(ctx context.Context, paymentID uuid.UUID) (model.Challenge, error) {
	chl := model.NewChallenge(paymentID)
	if err := service.chlRepo.Upsert(ctx, service.Datastore.RawDB(), chl); err != nil {
		return model.Challenge{}, fmt.Errorf("error creating challenge: %w", err)
	}
	return chl, nil
}

// DisconnectCustodianLink - removes the link to the custodian wallet that is active
func (service *Service) DisconnectCustodianLink(ctx context.Context, _ string, walletID uuid.UUID) error {
	if err := service.Datastore.DisconnectCustodialWallet(ctx, walletID); err != nil {
		return handlers.WrapError(err, "unable to disconnect custodian wallet", http.StatusInternalServerError)
	}
	return nil
}

// CreateRewardsWallet creates a brave rewards wallet and informs the reputation service.
// If either the local transaction or call to the reputation service fails then the wallet is not created.
func (service *Service) CreateRewardsWallet(ctx context.Context, publicKey string, geoCountry string) (*walletutils.Info, error) {
	log := logging.Logger(ctx, "wallets.CreateRewardsWallet")

	valid, err := service.geoValidator.Validate(ctx, geoCountry)
	if err != nil {
		return nil, fmt.Errorf("error validating geo country: %w", err)
	}

	if !valid {
		return nil, errGeoCountryDisabled
	}

	var altCurrency = altcurrency.BAT
	var info = &walletutils.Info{
		ID:          uuid.NewV5(ClaimNamespace, publicKey).String(),
		Provider:    "brave",
		PublicKey:   publicKey,
		AltCurrency: &altCurrency,
	}

	ctx, tx, rollback, commit, err := getTx(ctx, service.Datastore)
	if err != nil {
		return nil, fmt.Errorf("error creating transaction: %w", err)
	}
	defer rollback()

	err = service.Datastore.InsertWalletTx(ctx, tx, info)
	if err != nil {
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" { // unique constraint violation
				if info != nil {
					log.Error().Err(err).Interface("info", info).
						Msg("error InsertWalletTx")
				}
				return nil, errRewardsWalletAlreadyExists
			}
		}
		return nil, fmt.Errorf("error inserting rewards wallet: %w", err)
	}

	upsertReputationSummary := func() (interface{}, error) {
		return nil, service.repClient.UpsertReputationSummary(ctx, info.ID, geoCountry)
	}

	_, err = service.retry(ctx, upsertReputationSummary, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return nil, fmt.Errorf("error calling reputation service: %w", err)
	}

	err = commit()
	if err != nil {
		return nil, fmt.Errorf("error comitting rewards wallet transaction: %w", err)
	}

	return info, nil
}

func (service *Service) SolanaAddToWaitlist(ctx context.Context, paymentID uuid.UUID) error {
	if err := solanaCanJoinWaitlist(ctx, service.Datastore, paymentID); err != nil {
		return err
	}

	now := time.Now().UTC()

	if err := service.solWaitlistRepo.Insert(ctx, service.Datastore.RawDB(), paymentID, now); err != nil {
		if !errors.Is(err, model.ErrSolAlreadyWaitlisted) {
			return err
		}
	}

	return nil
}

type cxLinkRepo interface {
	GetCustodianLinkByWalletID(ctx context.Context, ID uuid.UUID) (*CustodianLink, error)
}

func solanaCanJoinWaitlist(ctx context.Context, repo cxLinkRepo, paymentID uuid.UUID) error {
	cxLink, err := repo.GetCustodianLinkByWalletID(ctx, paymentID)
	if err != nil && !errors.Is(err, model.ErrNoWalletCustodian) {
		return err
	}

	if cxLink.isLinked() && cxLink.isSolana() {
		return model.ErrSolAlreadyLinked
	}

	return nil
}

func (service *Service) SolanaDeleteFromWaitlist(ctx context.Context, paymentID uuid.UUID) error {
	return service.solWaitlistRepo.Delete(ctx, service.Datastore.RawDB(), paymentID)
}

// RefreshCustodianRegionsWorker - get the custodian regions from the merge param bucket
func (service *Service) RefreshCustodianRegionsWorker(ctx context.Context) (bool, error) {
	useCustodianRegions, featureOK := ctx.Value(appctx.UseCustodianRegionsCTXKey).(bool)
	if featureOK && !useCustodianRegions {
		// do not attempt no error
		return false, nil
	}
	// get aws client
	client, clientOK := ctx.Value(appctx.AWSClientCTXKey).(*appaws.Client)
	if !clientOK {
		return true, errors.New("cannot run refresh custodian regions, no client")
	}
	// get the bucket and if we are feature flagged on
	bucket, bucketOK := ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	if !bucketOK {
		return true, errors.New("cannot run refresh custodian regions, no bucket")
	}

	select {
	case <-ctx.Done():
		return true, ctx.Err()
	default:
		// use client to put the custodian regions on ctx
		custodianRegions, err := custodian.ExtractCustodianRegions(ctx, client, bucket)
		if err != nil {
			return true, fmt.Errorf("error running refresh custodian regions: %w", err)
		}
		// write custodian regions to service
		service.SetCustodianRegions(*custodianRegions)
		return true, nil
	}
}

func (service *Service) RunVerifiedWalletWorker(ctx context.Context) (bool, error) {
	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
			_, err := service.Datastore.SendVerifiedWalletOutbox(ctx, service.repClient, service.retry)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return true, nil
				}
				return true, fmt.Errorf("error running send verified wallet request: %w", err)
			}
		}
	}
}

func canRetry(nonRetriableErrors []int) func(error) bool {
	return func(err error) bool {
		var eb *errorutils.ErrorBundle
		switch {
		case errors.As(err, &eb):
			if hs, ok := eb.Data().(clients.HTTPState); ok {
				for _, httpStatusCode := range nonRetriableErrors {
					if hs.Status == httpStatusCode {
						return false
					}
				}
				return true
			}
		}
		return false
	}
}

type claimsZP struct {
	Iat         int64  `json:"iat"`
	Exp         int64  `json:"exp"`
	DepositID   string `json:"depositId"`
	AccountID   string `json:"accountId"`
	Valid       bool   `json:"isValid"`
	CountryCode string `json:"countryCode"`
}

func (c *claimsZP) validate(now time.Time) error {
	if c.Iat <= 0 {
		return errZPInvalidIat
	}

	if c.Exp <= 0 {
		return errZPInvalidExp
	}

	if !c.isKYC() {
		return errZPInvalidKYC
	}

	// Make sure deposit id exists
	if c.DepositID == "" {
		return errZPInvalidDepositID
	}

	// Get the account id.
	if c.AccountID == "" {
		return errZPInvalidAccountID
	}

	if !strings.EqualFold(c.CountryCode, "IN") {
		return errorutils.ErrInvalidCountry
	}

	return c.validateTime(now)
}

func (c *claimsZP) isKYC() bool {
	return c.Valid
}

func (c *claimsZP) validateTime(now time.Time) error {
	if now.Before(time.Unix(c.Iat, 0)) {
		return errZPInvalidAfter
	}

	if now.After(time.Unix(c.Exp, 0)) {
		return errZPInvalidBefore
	}

	return nil
}

func isWalletWhitelisted(ctx context.Context, dbi sqlx.QueryerContext, alRepo allowListRepo, paymentID uuid.UUID) error {
	if _, err := alRepo.GetAllowListEntry(ctx, dbi, paymentID); err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return model.ErrWalletNotWhitelisted
		}
		return fmt.Errorf("error checking allow list entry: %w", err)
	}
	return nil
}

const (
	errZPParseToken       model.Error = "zebpay linking info parsing failed"
	errZPNoHeaders        model.Error = "linking info token invalid no headers"
	errZPInvalidToken     model.Error = "linking info token invalid"
	errZPValidationFailed model.Error = "zebpay linking info validation failed"
)

func parseZebPayClaims(ctx context.Context, verificationToken string) (claimsZP, error) {
	const msgBadConf = "zebpay linking validation misconfigured"
	linkingKeyB64, ok := ctx.Value(appctx.ZebPayLinkingKeyCTXKey).(string)
	if !ok {
		return claimsZP{}, handlers.WrapError(appctx.ErrNotInContext, msgBadConf, http.StatusInternalServerError)
	}

	decodedJWTKey, err := base64.StdEncoding.DecodeString(linkingKeyB64)
	if err != nil {
		return claimsZP{}, handlers.WrapError(appctx.ErrNotInContext, msgBadConf, http.StatusInternalServerError)
	}

	tok, err := jwt.ParseSigned(verificationToken)
	if err != nil {
		return claimsZP{}, handlers.WrapError(errZPParseToken, errZPParseToken.Error(), http.StatusBadRequest)
	}

	if len(tok.Headers) == 0 {
		return claimsZP{}, handlers.WrapError(errZPNoHeaders, errZPNoHeaders.Error(), http.StatusBadRequest)
	}

	for i := range tok.Headers {
		if tok.Headers[i].Algorithm != "HS256" {
			return claimsZP{}, handlers.WrapError(errZPInvalidToken, errZPInvalidToken.Error(), http.StatusBadRequest)
		}
	}

	var claims claimsZP
	if err := tok.Claims(decodedJWTKey, &claims); err != nil {
		return claimsZP{}, handlers.WrapError(errZPValidationFailed, errZPValidationFailed.Error(), http.StatusBadRequest)
	}

	return claims, nil
}

type deleter interface {
	DeleteAfter(ctx context.Context, dbi sqlx.ExecerContext, interval time.Duration) error
}

type deleteExpiredChallengeTask struct {
	exec           sqlx.ExecerContext
	deleter        deleter
	deleteAfterMin time.Duration
}

func (d *deleteExpiredChallengeTask) deleteExpiredChallenges(ctx context.Context) (bool, error) {
	if err := d.deleter.DeleteAfter(ctx, d.exec, d.deleteAfterMin); err != nil && !errors.Is(err, model.ErrNoRowsDeleted) {
		return false, fmt.Errorf("error deleting expired challenges: %w", err)
	}
	return true, nil
}
