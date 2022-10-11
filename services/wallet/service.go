package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"

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
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/go-chi/chi"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

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
)

type GeoValidator interface {
	Validate(ctx context.Context, gelocation string) (bool, error)
}

// Service contains datastore connections
type Service struct {
	Datastore    Datastore
	RoDatastore  ReadOnlyDatastore
	repClient    reputation.Client
	geminiClient gemini.Client
	geoValidator GeoValidator
	retry        backoff.RetryFunc
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore, repClient reputation.Client,
	geminiClient gemini.Client, geoCountryValidator GeoValidator,
	retry backoff.RetryFunc) (*Service, error) {
	service := &Service{
		Datastore:    datastore,
		RoDatastore:  roDatastore,
		repClient:    repClient,
		geminiClient: geminiClient,
		geoValidator: geoCountryValidator,
		retry:        retry,
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

	bucket, bucketOK := ctx.Value(appctx.ParametersMergeBucketCTXKey).(string)
	useCustodianRegions, featureOK := ctx.Value(appctx.UseCustodianRegionsCTXKey).(bool)
	if featureOK && useCustodianRegions && !bucketOK {
		logger.Panic().Msg("failed to initialize wallet service, misconfiguration for custodian regions bucket")
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

	if useCustodianRegions {
		// use client to put the custodian regions on ctx
		custodianRegions, err := custodian.ExtractCustodianRegions(ctx, awsClient, bucket)
		if err != nil {
			logger.Panic().Err(err).Msg("failed to initialize wallet service, unable to extract custodian regions")
		}
		ctx = context.WithValue(ctx, appctx.CustodianRegionsCTXKey, custodianRegions)
	}

	return ctx, s
}

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
			r.Post("/brave/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkBraveDepositAccount", LinkBraveDepositAccountV3(s))).ServeHTTP)
			r.Post("/gemini/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkGeminiDepositAccount", LinkGeminiDepositAccountV3(s))).ServeHTTP)
			// disconnect verified custodial wallet
			r.Delete("/{custodian}/{paymentID}/claim", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"DisconnectCustodianLinkV3", DisconnectCustodianLinkV3(s))).ServeHTTP)

			// create wallet connect routes for our wallet providers
			r.Post("/uphold/{paymentID}/connect", middleware.InstrumentHandlerFunc(
				"LinkUpholdDepositAccount", LinkUpholdDepositAccountV3(s)))
			r.Post("/bitflyer/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkBitFlyerDepositAccount", LinkBitFlyerDepositAccountV3(s))).ServeHTTP)
			r.Post("/brave/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkBraveDepositAccount", LinkBraveDepositAccountV3(s))).ServeHTTP)
			r.Post("/gemini/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"LinkGeminiDepositAccount", LinkGeminiDepositAccountV3(s))).ServeHTTP)
			// disconnect verified custodial wallet
			r.Delete("/{custodian}/{paymentID}/connect", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
				"DisconnectCustodianLinkV3", DisconnectCustodianLinkV3(s))).ServeHTTP)
		}

		// unlink verified custodial wallet
		r.Delete("/{custodian}/{payment_id}/unlink", middleware.SimpleTokenAuthorizedOnly(
			middleware.InstrumentHandlerFunc("UnlinkWallet", UnlinkWalletV3(s))).ServeHTTP)

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
		r.Post("/", middleware.InstrumentHandlerFunc("CreateWalletV4", CreateWalletV4(s)))
		r.Patch("/{paymentID}", middleware.HTTPSignedOnly(s)(middleware.InstrumentHandlerFunc(
			"UpdateWalletV4", UpdateWalletV4(s))).ServeHTTP)
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

// UnlinkWallet - unlink this wallet from the custodian
func (service *Service) UnlinkWallet(ctx context.Context, walletID, custodian string) error {
	id, err := uuid.FromString(walletID)
	if err != nil {
		return fmt.Errorf("unable to parse wallet id: %w", err)
	}
	switch custodian {
	case "uphold", "bitflyer", "gemini", "brave":
	default:
		return fmt.Errorf("custodian '%s' not valid", custodian)
	}

	if err := service.Datastore.UnlinkWallet(ctx, id, custodian); err != nil {
		return fmt.Errorf("unable to unlink wallet: %w", err)
	}
	return nil
}

// IncreaseLinkingLimit - increase this wallet's linking limit
func (service *Service) IncreaseLinkingLimit(ctx context.Context, custodianID string) error {
	// convert to provider id
	providerLinkingID := uuid.NewV5(ClaimNamespace, custodianID)

	if err := service.Datastore.IncreaseLinkingLimit(ctx, providerLinkingID); err != nil {
		return fmt.Errorf("unable to increase linking limit: %w", err)
	}
	return nil
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
func (service *Service) LinkBitFlyerWallet(ctx context.Context, walletID uuid.UUID, depositID, accountHash string) error {
	// during validation, we verified that the account hash and deposit id were signed by bitflyer
	// we also validated that this "info" signed the request to perform the linking with http signature
	// we assume that since we got linkingInfo signed from BF that they are KYC
	providerLinkingID := uuid.NewV5(ClaimNamespace, accountHash)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err := service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, "bitflyer", "JP")
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		if errors.Is(err, ErrUnusualActivity) {
			return handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}
		if errors.Is(err, ErrGeoResetDifferent) {
			// custodial account country code does not match declared country code
			return handlers.AppError{
				Message: "custodial account country code does not match declared country code",
				Code:    http.StatusBadRequest,
			}
		}
		return handlers.WrapError(err, "unable to link bitflyer wallets", status)
	}
	return nil
}

// LinkGeminiWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkGeminiWallet(ctx context.Context, walletID uuid.UUID, verificationToken, depositID string) error {
	// get gemini client from context
	geminiClient, ok := ctx.Value(appctx.GeminiClientCTXKey).(gemini.Client)
	if !ok {
		// no gemini client on context
		return handlers.WrapError(
			appctx.ErrNotInContext, "gemini client misconfigured", http.StatusInternalServerError)
	}

	// perform an Account Validation call to gemini to get the accountID
	accountID, country, err := geminiClient.ValidateAccount(ctx, verificationToken, depositID)
	if err != nil {
		// check if this gemini accountID has already been linked to this wallet,
		if errors.Is(err, errorutils.ErrInvalidCountry) {
			ok, err := service.Datastore.HasPriorLinking(
				ctx, walletID, uuid.NewV5(ClaimNamespace, accountID))
			if err != nil {
				return fmt.Errorf("failed to check prior linkings: %w", err)
			}
			if !ok {
				// then pass back the original geo error
				return fmt.Errorf("failed to validate account: %w", err)
			}
			// allow invalid country if there was a prior linking
		} else {
			// if so ignore the region not supported error
			return fmt.Errorf("failed to validate account: %w", err)
		}
	}

	// we assume that since we got linking_info(VerificationToken) signed from Gemini that they are KYC
	providerLinkingID := uuid.NewV5(ClaimNamespace, accountID)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err = service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, "gemini", country)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		if errors.Is(err, ErrUnusualActivity) {
			return handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}
		if errors.Is(err, ErrGeoResetDifferent) {
			// custodial account country code does not match declared country code
			return handlers.AppError{
				Message: "custodial account country code does not match declared country code",
				Code:    http.StatusBadRequest,
			}
		}
		return handlers.WrapError(err, "unable to link gemini wallets", status)
	}
	return nil
}

// LinkWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkWallet(
	ctx context.Context,
	wallet uphold.Wallet,
	transaction string,
	anonymousAddress *uuid.UUID,
) error {
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
		return handlers.WrapError(
			errors.New("failed to verify transaction"), "transaction verification failure",
			http.StatusForbidden)
	}

	// verify that the user is kyc from uphold. (for all wallet provider cases)
	if uID, ok, c, err := wallet.IsUserKYC(ctx, transactionInfo.Destination); err != nil {
		// get the rewards wallet id from the uphold wallet info
		infoID, err := uuid.FromString(info.ID)
		if err != nil {
			return fmt.Errorf("failed to parse wallet id: %w", err)
		}
		// check if this gemini accountID has already been linked to this wallet,
		if errors.Is(err, errorutils.ErrInvalidCountry) {
			ok, err := service.Datastore.HasPriorLinking(
				ctx, infoID, uuid.NewV5(ClaimNamespace, userID))
			if err != nil {
				return fmt.Errorf("failed to check prior linkings: %w", err)
			}
			if !ok {
				// then pass back the original geo error
				return err
			}
			// allow invalid country if there was a prior linking
		} else {
			// if so ignore the region not supported error
			return err
		}
		// there was an unexpected error
		return handlers.WrapError(err,
			"wallet could not be kyc checked",
			http.StatusInternalServerError,
		)
	} else if !ok {
		// fail
		return handlers.WrapError(
			errors.New("user kyc did not pass"),
			"KYC required",
			http.StatusForbidden)
	} else {
		userID = uID
		country = c
	}

	// check kyc user id validity
	if userID == "" {
		return handlers.WrapError(
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
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		if errors.Is(err, ErrUnusualActivity) {
			return handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}
		if errors.Is(err, ErrGeoResetDifferent) {
			// custodial account country code does not match declared country code
			return handlers.AppError{
				Message: "custodial account country code does not match declared country code",
				Code:    http.StatusBadRequest,
			}
		}
		return handlers.WrapError(err, "unable to link uphold wallets", status)
	}

	// if this wallet is linking a deposit account do not submit a transaction
	if decimal.NewFromFloat(0).LessThan(probi) {
		_, err := service.SubmitCommitableAnonCardTransaction(ctx, &info, transaction, "", true)
		if err != nil {
			return handlers.WrapError(err, "unable to transfer tokens", http.StatusBadRequest)
		}
	}
	return nil
}

// LinkBraveWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkBraveWallet(ctx context.Context, from, to uuid.UUID) error {

	// get reputation client from context
	repClient, ok := ctx.Value(appctx.ReputationClientCTXKey).(reputation.Client)
	if !ok {
		return fmt.Errorf("server misconfigured: no reputation client")
	}

	// is this from wallet reputable as an iOS device?
	isFromOnPlatform, err := repClient.IsWalletOnPlatform(ctx, from, "ios")
	if err != nil {
		return fmt.Errorf("invalid device: %w", err)
	}

	if !isFromOnPlatform {
		// wallet is not reputable, decline
		return fmt.Errorf("unable to link wallet: invalid device")
	}

	// link the wallet in our datastore, provider linking id will be on the deposit wallet (to wallet)
	providerLinkingID := uuid.NewV5(ClaimNamespace, to.String())

	// "to" will be stored as UserDepositDestination in the wallet info upon linking
	if err := service.Datastore.LinkWallet(ctx, from.String(), to.String(), providerLinkingID, nil, "brave", ""); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			// we are not allowing draining to wallets that exceed the linking limits
			// this will cause an error in the client prior to attempting draining
			status = http.StatusTeapot
		}
		if errors.Is(err, ErrUnusualActivity) {
			return handlers.WrapError(err, "unable to link - unusual activity", http.StatusBadRequest)
		}
		if errors.Is(err, ErrGeoResetDifferent) {
			// custodial account country code does not match declared country code
			return handlers.AppError{
				Message: "custodial account country code does not match declared country code",
				Code:    http.StatusBadRequest,
			}
		}
		return handlers.WrapError(err, "unable to link brave wallets", status)
	}

	return nil
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
