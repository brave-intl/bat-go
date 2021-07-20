package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	"github.com/brave-intl/bat-go/utils/clients/reputation"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
)

var (
	// WalletClaimNamespace uuidv5 namespace for provider linking - exported for tests
	WalletClaimNamespace = uuid.Must(uuid.FromString("c39b298b-b625-42e9-a463-69c7726e5ddc"))
)

// Service contains datastore connections
type Service struct {
	Datastore    Datastore
	RoDatastore  ReadOnlyDatastore
	repClient    reputation.Client
	geminiClient gemini.Client
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(ctx context.Context, datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	service := &Service{
		Datastore:   datastore,
		RoDatastore: roDatastore,
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
	_, err = anonCard.VerifyAnonCardTransaction(transaction, destination)
	if err != nil {
		return nil, err
	}

	// Submit and confirm since we are requiring the idempotency key
	return anonCard.SubmitTransaction(transaction, confirm)
}

// IncreaseLinkingLimit - increase this wallet's linking limit
func (service *Service) IncreaseLinkingLimit(ctx context.Context, custodianID string) error {
	// convert to provider id
	providerLinkingID := uuid.NewV5(WalletClaimNamespace, custodianID)

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
		providerLinkingID = uuid.NewV5(WalletClaimNamespace, custodianID).String()
	}

	infos, err := service.Datastore.GetLinkingLimitInfo(ctx, providerLinkingID)
	if err != nil {
		return infos, fmt.Errorf("unable to increase linking limit: %w", err)
	}
	return infos, nil
}

// LinkBitFlyerWallet links a wallet and transfers funds to newly linked wallet
func (service *Service) LinkBitFlyerWallet(ctx context.Context, walletID uuid.UUID, depositID, accountHash string) error {
	// during validation we verified that the account hash and deposit id were signed by bitflyer
	// we also validated that this "info" signed the request to perform the linking with http signature
	// we assume that since we got linkingInfo signed from BF that they are KYC
	providerLinkingID := uuid.NewV5(WalletClaimNamespace, accountHash)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err := service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, "bitflyer")
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		return handlers.WrapError(err, "unable to link wallets", status)
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
	accountID, err := geminiClient.ValidateAccount(ctx, verificationToken, depositID)
	if err != nil {
		return handlers.WrapError(
			errors.New("invalid linking_info"), "unable to validate gemini account", http.StatusBadRequest)
	}

	// we assume that since we got linking_info(VerificationToken) signed from Gemini that they are KYC
	providerLinkingID := uuid.NewV5(WalletClaimNamespace, accountID)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err = service.Datastore.LinkWallet(ctx, walletID.String(), depositID, providerLinkingID, nil, "gemini")
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		return handlers.WrapError(err, "unable to link wallets", status)
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
		depositProvider string
		probi           decimal.Decimal
	)

	tx, err := wallet.VerifyTransaction(transaction)
	if err != nil {
		return handlers.WrapError(errors.New("failed to verify transaction"), "failed to verify transaction", http.StatusForbidden)
	}

	// verify that the user is kyc from uphold. (for all wallet provider cases)
	if uID, ok, err := wallet.IsUserKYC(ctx, tx.Destination); err != nil {
		// there was an error
		return handlers.WrapError(err,
			"wallet could not be kyc checked",
			http.StatusInternalServerError,
		)
	} else if !ok {
		// fail
		return &handlers.AppError{
			Cause: errors.New("user kyc did not pass"),
			Code:  http.StatusForbidden,
		}
	} else {
		userID = uID
	}

	// check kyc user id validity
	if userID == "" {
		err := errors.New("user id not provided")
		return handlers.WrapError(err, "unable to link wallet", http.StatusBadRequest)
	}

	probi = tx.Probi
	depositProvider = "uphold"

	providerLinkingID := uuid.NewV5(WalletClaimNamespace, userID)
	// tx.Destination will be stored as UserDepositDestination in the wallet info upon linking
	err = service.Datastore.LinkWallet(ctx, info.ID, tx.Destination, providerLinkingID, anonymousAddress, depositProvider)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			status = http.StatusConflict
		}
		return handlers.WrapError(err, "unable to link wallets", status)
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

// SetupService - setup the wallet microservice
func SetupService(ctx context.Context, r *chi.Mux) (*chi.Mux, context.Context, *Service) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	// setup the service now
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

	s, err := InitService(ctx, db, roDB)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize wallet service")
	}

	// setup reputation client
	s.repClient, err = reputation.New()
	// its okay to not fatally fail if this environment is local and we cant make a rep client
	if err != nil && os.Getenv("ENV") != "local" {
		logger.Fatal().Err(err).Msg("failed to initialize wallet service")
	}

	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, s.repClient)

	if os.Getenv("GEMINI_ENABLED") == "true" {
		s.geminiClient, err = gemini.New()
		if err != nil {
			logger.Panic().Err(err).Msg("failed to create gemini client")
		}
		ctx = context.WithValue(ctx, appctx.GeminiClientCTXKey, s.geminiClient)
	}

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
		}
		// support only APIs to assist in linking limit issues
		/*
			TODO: currently commented out due to concerns about how to enable/disable particular
			people from accessing this endpoint.
			r.Post("/{custodian}/increase-limit/{custodian_id}", middleware.SimpleTokenAuthorizedOnly(
				middleware.InstrumentHandlerFunc("IncreaseLinkingLimit", IncreaseLinkingLimitV3(s))).ServeHTTP)
		*/

		// linking info api is okay to expose publically
		r.Get("/linking-info",
			middleware.InstrumentHandlerFunc("GetLinkingInfo", GetLinkingInfoV3(s)).ServeHTTP)

		// get wallet routes
		r.Get("/{paymentID}", middleware.InstrumentHandlerFunc(
			"GetWallet", GetWalletV3))
		r.Get("/recover/{publicKey}", middleware.InstrumentHandlerFunc(
			"RecoverWallet", RecoverWalletV3))

		// get wallet balance routes
		r.Get("/uphold/{paymentID}", middleware.InstrumentHandlerFunc(
			"GetUpholdWalletBalance", GetUpholdWalletBalanceV3))
	})
	return r, ctx, s
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
	providerLinkingID := uuid.NewV5(WalletClaimNamespace, to.String())

	// "to" will be stored as UserDepositDestination in the wallet info upon linking
	if err := service.Datastore.LinkWallet(ctx, from.String(), to.String(), providerLinkingID, nil, "brave"); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrTooManyCardsLinked) {
			// we are not allowing draining to wallets that exceed the linking limits
			// this will cause an error in the client prior to attempting draining
			status = http.StatusTeapot
		}
		return handlers.WrapError(err, "unable to link wallets", status)
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
