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

	"github.com/go-chi/chi"
	"github.com/go-jose/go-jose/v3/jwt"
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

// Service contains datastore connections
type Service struct {
	Datastore        Datastore
	RoDatastore      ReadOnlyDatastore
	repClient        reputation.Client
	geminiClient     gemini.Client
	geoValidator     GeoValidator
	retry            backoff.RetryFunc
	jobs             []srv.Job
	crMu             *sync.RWMutex
	custodianRegions custodian.Regions
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore, repClient reputation.Client,
	geminiClient gemini.Client, geoCountryValidator GeoValidator,
	retry backoff.RetryFunc) (*Service, error) {
	service := &Service{
		crMu:         new(sync.RWMutex),
		Datastore:    datastore,
		RoDatastore:  roDatastore,
		repClient:    repClient,
		geminiClient: geminiClient,
		geoValidator: geoCountryValidator,
		retry:        retry,
	}
	// get the valid custodian regions
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
	logger := logging.Logger(ctx, "wallet.SetupService")

	db, err := NewWritablePostgres(viper.GetString("datastore"), false, "wallet_db")
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to wallet db")
	}

	roDB, err := NewReadOnlyPostgres(viper.GetString("ro-datastore"), false, "wallet_ro_db")
	if err != nil {
		logger.Panic().Err(err).Msg("unable connect to wallet db")
	}

	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDB)
	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, db)

	// add our command line params to context
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, viper.Get("environment"))

	// jwt key is hex encoded string
	decodedBitFlyerJWTKey, err := hex.DecodeString(viper.GetString("bitflyer-jwt-key"))
	if err != nil {
		logger.Error().Err(err).Msg("invalid bitflyer jwt key")
	}
	ctx = context.WithValue(ctx, appctx.BitFlyerJWTKeyCTXKey, decodedBitFlyerJWTKey)

	// setup reputation client
	repClient, err := reputation.New()
	// it's okay to not fatally fail if this environment is local and we cant make a rep client
	if err != nil && os.Getenv("ENV") != "local" {
		logger.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, repClient)

	var geminiClient gemini.Client
	if os.Getenv("GEMINI_ENABLED") == "true" {
		geminiClient, err = gemini.New()
		if err != nil {
			logger.Panic().Err(err).Msg("failed to create gemini client")
		}
		ctx = context.WithValue(ctx, appctx.GeminiClientCTXKey, geminiClient)
	}

	cfg, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	awsClient, err := appaws.NewClient(cfg)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	// put the configured aws client on ctx
	ctx = context.WithValue(ctx, appctx.AWSClientCTXKey, awsClient)

	// get the s3 bucket and object
	bucket, bucketOK := ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	if !bucketOK {
		logger.Panic().Err(errors.New("bucket not in context")).
			Msg("failed to initialize wallet service")
	}
	object, ok := ctx.Value(appctx.DisabledWalletGeoCountriesCTXKey).(string)
	if !ok {
		logger.Panic().Err(errors.New("wallet geo countries disabled ctx key value not found")).
			Msg("failed to initialize wallet service")
	}

	config := Config{
		bucket: bucket,
		object: object,
	}

	geoCountryValidator := NewGeoCountryValidator(awsClient, config)

	s, err := InitService(db, roDB, repClient, geminiClient, geoCountryValidator, backoff.Retry)
	if err != nil {
		logger.Panic().Err(err).Msg("failed to initialize wallet service")
	}

	_, err = s.RefreshCustodianRegionsWorker(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("failed to initialize custodian regions")
	}

	s.jobs = []srv.Job{
		{
			Func:    s.RefreshCustodianRegionsWorker,
			Cadence: 15 * time.Minute,
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
		logger.Error().Err(err).Msg("error initializing job workers")
	}

	return ctx, s
}

func (service *Service) setCustodianRegions(custodianRegions custodian.Regions) {
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
func RegisterRoutes(ctx context.Context, s *Service, r *chi.Mux) *chi.Mux {
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
		}

		r.Get("/linking-info", middleware.SimpleTokenAuthorizedOnly(
			middleware.InstrumentHandlerFunc("GetLinkingInfo", GetLinkingInfoV3(s))).ServeHTTP)

		// get wallet routes
		r.Get("/{paymentID}", middleware.InstrumentHandlerFunc(
			"GetWallet", GetWalletV3))
		r.Get("/recover/{publicKey}", middleware.InstrumentHandlerFunc(
			"RecoverWallet", RecoverWalletV3))

		// get wallet balance routes
		r.Get("/uphold/{paymentID}", middleware.InstrumentHandlerFunc(
			"GetUpholdWalletBalance", GetUpholdWalletBalanceV3))
	})

	r.Route("/v4/wallets", func(r chi.Router) {
		r.Use(middleware.RateLimiter(ctx, 2))
		r.Post("/", middleware.InstrumentHandlerFunc("CreateWalletV4", CreateWalletV4(s)))
		r.Patch("/{paymentID}", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
			"UpdateWalletV4", UpdateWalletV4(s))).ServeHTTP)
		r.Get("/{paymentID}",
			middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"GetWalletV4", GetWalletV4)).ServeHTTP)
		// get wallet balance routes
		r.Get("/uphold/{paymentID}",
			middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"GetUpholdWalletBalanceV4", GetUpholdWalletBalanceV4)).ServeHTTP)
	})

	return r
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

	err := checkCustodianLinkingMismatch(ctx, service.Datastore, walletID, depositProvider)
	if err != nil {
		if errors.Is(err, errCustodianLinkMismatch) {
			return "", errCustodianLinkMismatch
		}
		return "", handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
	}

	// In the controller validation, we verified that the account hash and deposit id were signed by bitflyer
	// we also validated that this "info" signed the request to perform the linking with http signature
	// we assume that since we got linkingInfo signed from BF that they are KYC
	providerLinkingID := uuid.NewV5(ClaimNamespace, accountHash)
	err = service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, depositProvider, country)
	if err != nil {
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
	const (
		depositProvider = "zebpay"
		country         = "IN"
	)

	// Get zebpay linking_info signing key.
	linkingKeyB64, ok := ctx.Value(appctx.ZebPayLinkingKeyCTXKey).(string)
	if !ok {
		const msg = "zebpay linking validation misconfigured"
		return "", handlers.WrapError(appctx.ErrNotInContext, msg, http.StatusInternalServerError)
	}

	// Decode base64 encoded jwt key.
	decodedJWTKey, err := base64.StdEncoding.DecodeString(linkingKeyB64)
	if err != nil {
		const msg = "zebpay linking validation misconfigured"
		return "", handlers.WrapError(appctx.ErrNotInContext, msg, http.StatusInternalServerError)
	}

	// Parse the signed verification token from input.
	tok, err := jwt.ParseSigned(verificationToken)
	if err != nil {
		const msg = "zebpay linking info parsing failed"
		return "", handlers.WrapError(appctx.ErrNotInContext, msg, http.StatusBadRequest)
	}

	if len(tok.Headers) == 0 {
		const msg = "linking info token invalid no headers"
		return "", handlers.WrapError(errors.New(msg), msg, http.StatusBadRequest)
	}

	// validate algorithm used
	for i := range tok.Headers {
		if tok.Headers[i].Algorithm != "HS256" {
			const msg = "linking info token invalid"
			return "", handlers.WrapError(errors.New(msg), msg, http.StatusBadRequest)
		}
	}

	// Create the jwt claims and get them (verified) from the token.
	claims := &claimsZP{}
	if err := tok.Claims(decodedJWTKey, claims); err != nil {
		const msg = "zebpay linking info validation failed"
		return "", handlers.WrapError(errors.New(msg), msg, http.StatusBadRequest)
	}

	if err := claims.validate(time.Now()); err != nil {
		return "", err
	}

	err = checkCustodianLinkingMismatch(ctx, service.Datastore, walletID, depositProvider)
	if err != nil {
		if errors.Is(err, errCustodianLinkMismatch) {
			return "", errCustodianLinkMismatch
		}
		return "", handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
	}

	providerLinkingID := uuid.NewV5(ClaimNamespace, claims.AccountID)
	if err := service.Datastore.LinkWallet(ctx, walletID.String(), claims.DepositID, providerLinkingID, nil, depositProvider, country); err != nil {
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

		return "", handlers.WrapError(err, "unable to link zebpay wallets", status)
	}

	return country, nil
}

// LinkGeminiWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkGeminiWallet(ctx context.Context, walletID uuid.UUID, verificationToken, depositID string) (string, error) {
	const depositProvider = "gemini"

	// get gemini client from context
	geminiClient, ok := ctx.Value(appctx.GeminiClientCTXKey).(gemini.Client)
	if !ok {
		// no gemini client on context
		return "", handlers.WrapError(appctx.ErrNotInContext, "gemini client misconfigured", http.StatusInternalServerError)
	}

	// add custodian regions to ctx going to client
	_, ok = ctx.Value(appctx.CustodianRegionsCTXKey).(custodian.Regions)
	if !ok {
		cr := service.getCustodianRegions()
		ctx = context.WithValue(ctx, appctx.CustodianRegionsCTXKey, &cr)
	}

	err := checkCustodianLinkingMismatch(ctx, service.Datastore, walletID, depositProvider)
	if err != nil {
		if errors.Is(err, errCustodianLinkMismatch) {
			return "", errCustodianLinkMismatch
		}
		return "", handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
	}

	// If a wallet has previously been linked i.e. has a prior linking, but the country is now invalid/blocked
	// then we can allow the account to link due to its prior successful linking i.e. it is grandfathered.
	// If there is no prior linking and the country is invalid/blocked then we should apply the current rules and block it.

	accountID, country, err := geminiClient.ValidateAccount(ctx, verificationToken, depositID)
	if err != nil {
		if errors.Is(err, errorutils.ErrInvalidCountry) {
			hasPriorLinking, priorLinkingErr := service.Datastore.HasPriorLinking(ctx, walletID, uuid.NewV5(ClaimNamespace, accountID))
			if priorLinkingErr != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", fmt.Errorf("failed to check prior linkings: %w", priorLinkingErr)
			}

			if !hasPriorLinking {
				return "", fmt.Errorf("failed to validate account: %w", err)
			}

		} else {
			// not err invalid country error
			return "", fmt.Errorf("failed to validate account: %w", err)
		}
	}

	// we assume that since we got linking_info(VerificationToken) signed from Gemini that they are KYC
	providerLinkingID := uuid.NewV5(ClaimNamespace, accountID)
	err = service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, depositProvider, country)
	if err != nil {
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

		return "", handlers.WrapError(err, "unable to link gemini wallets", status)
	}

	return country, nil
}

// LinkUpholdWallet links an uphold.Wallet and transfers funds.
func (service *Service) LinkUpholdWallet(ctx context.Context, wallet uphold.Wallet, transaction string, anonymousAddress *uuid.UUID) (string, error) {
	// do not confirm this transaction yet
	info := wallet.GetWalletInfo()

	var (
		userID          string
		country         string
		depositProvider string
		probi           decimal.Decimal
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

	err = checkCustodianLinkingMismatch(ctx, service.Datastore, walletID, depositProvider)
	if err != nil {
		if errors.Is(err, errCustodianLinkMismatch) {
			return "", errCustodianLinkMismatch
		}
		return "", handlers.WrapError(err, "failed to check linking mismatch", http.StatusInternalServerError)
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
	depositProvider = "uphold"

	providerLinkingID := uuid.NewV5(ClaimNamespace, userID)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err = service.Datastore.LinkWallet(ctx, info.ID, transactionInfo.Destination, providerLinkingID, anonymousAddress, depositProvider, country)
	if err != nil {
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

// DisconnectCustodianLink - removes the link to the custodian wallet that is active
func (service *Service) DisconnectCustodianLink(ctx context.Context, custodian string, walletID uuid.UUID) error {
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
		service.setCustodianRegions(*custodianRegions)
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

func checkCustodianLinkingMismatch(ctx context.Context, storage Datastore, walletID uuid.UUID, depositProvider string) error {
	c, err := storage.GetCustodianLinkByWalletID(ctx, walletID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	// if there are no instances of wallet custodian then it is
	// considered a new linking and therefore valid.
	if c == nil {
		return nil
	}

	if !strings.EqualFold(c.Custodian, depositProvider) {
		return errCustodianLinkMismatch
	}

	return nil
}
