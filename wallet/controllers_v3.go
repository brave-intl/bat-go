package wallet

import (
	"crypto"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// ------------------ V3 Wallet APIs ---------------

// CreateUpholdWalletV3 - produces an http handler for the service s which handles creation of uphold wallets
func CreateUpholdWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var (
		ucReq       = new(UpholdCreationRequest)
		ctx         = r.Context()
		altCurrency = altcurrency.BAT
	)

	// no logger, setup
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	// decode and validate the request body
	if err := inputs.DecodeAndValidateReader(ctx, ucReq, r.Body); err != nil {
		return ucReq.HandleErrors(err)
	}

	// get public key from the input signed Creation Request
	var publicKey = ucReq.PublicKey

	// no more uphold wallets in the wild please
	if env, ok := ctx.Value(appctx.EnvironmentCTXKey).(string); ok && env == "local" {
		return handlers.WrapError(
			errors.New("uphold wallet creation needs to be in an environment not local"),
			"failed to create wallet", http.StatusBadRequest)
	}

	var (
		db Datastore
		ok bool
	)

	// get datastore from context
	if db, ok = ctx.Value(appctx.DatastoreCTXKey).(Datastore); !ok {
		logger.Error().Msg("unable to get datastore from context")
		return handlers.WrapError(err, "misconfigured datastore", http.StatusServiceUnavailable)
	}

	var info = &walletutils.Info{
		ID:          uuid.NewV4().String(),
		Provider:    "uphold",
		PublicKey:   publicKey,
		AltCurrency: &altCurrency,
	}

	uwallet := uphold.Wallet{
		Info:    *info,
		PrivKey: ed25519.PrivateKey{},
		PubKey:  httpsignature.Ed25519PubKey([]byte(publicKey)),
	}
	if err := uwallet.SubmitRegistration(ctx, ucReq.SignedCreationRequest); err != nil {
		return handlers.WrapError(
			errors.New("unable to create uphold wallet"),
			"failed to register wallet with uphold", http.StatusServiceUnavailable)
	}
	info.ProviderID = uwallet.GetWalletInfo().ProviderID

	// get wallet from datastore
	err = db.InsertWallet(ctx, info)
	if err != nil {
		logger.Error().Err(err).Str("id", info.ID).Msg("unable to create uphold wallet")
		return handlers.WrapError(err, "error writing wallet to storage", http.StatusServiceUnavailable)
	}

	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusCreated)
}

// CreateBraveWalletV3 - produces an http handler for the service s which handles creation of brave wallets
func CreateBraveWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	verifier := httpsignature.ParameterizedKeystoreVerifier{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Keystore: &DecodeEd25519Keystore{},
		Opts:     crypto.Hash(0),
	}

	// perform validation based on public key that the user submits
	ctx, keyID, err := verifier.VerifyRequest(r)
	if err != nil {
		return handlers.WrapError(err, "invalid http signature", http.StatusForbidden)
	}

	var (
		bcr = new(BraveCreationRequest)
	)

	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	if err := inputs.DecodeAndValidateReader(ctx, bcr, r.Body); err != nil {
		return bcr.HandleErrors(err)
	}

	var (
		db Datastore
		ok bool
	)

	// get datastore from context
	if db, ok = ctx.Value(appctx.DatastoreCTXKey).(Datastore); !ok {
		logger.Error().Msg("unable to get datastore from context")
		return handlers.WrapError(err, "misconfigured datastore", http.StatusServiceUnavailable)
	}

	var altCurrency = altcurrency.BAT

	var info = &walletutils.Info{
		ID:          uuid.NewV4().String(),
		Provider:    "brave",
		PublicKey:   keyID,
		AltCurrency: &altCurrency,
	}

	// get wallet from datastore
	err = db.InsertWallet(ctx, info)
	if err != nil {
		logger.Error().Err(err).Str("id", info.ID).Msg("unable to create brave wallet")
		return handlers.WrapError(err, "error writing wallet to storage", http.StatusServiceUnavailable)
	}

	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusCreated)
}

// LinkBitFlyerDepositAccountV3 - produces an http handler for the service s which handles deposit account linking of uphold wallets
func LinkBitFlyerDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			blr = new(BitFlyerLinkingRequest)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// validate payment id matches what was in the http signature
		signatureID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		if id.String() != signatureID {
			return handlers.ValidationError(
				"paymentId from URL does not match paymentId in http signature",
				map[string]interface{}{
					"paymentID": "does not match http signature id",
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(ctx, blr, r.Body); err != nil {
			return blr.HandleErrors(err)
		}

		err = s.LinkBitFlyerWallet(ctx, *id.UUID(), blr.DepositID, blr.AccountHash)
		if err != nil {
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// LinkGeminiDepositAccountV3 - produces an http handler for the service s which handles deposit account linking of uphold wallets
func LinkGeminiDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			glr = new(GeminiLinkingRequest)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// validate payment id matches what was in the http signature
		signatureID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		if id.String() != signatureID {
			return handlers.ValidationError(
				"paymentId from URL does not match paymentId in http signature",
				map[string]interface{}{
					"paymentID": "does not match http signature id",
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(ctx, glr, r.Body); err != nil {
			return glr.HandleErrors(err)
		}

		err = s.LinkGeminiWallet(ctx, *id.UUID(), glr.VerificationToken, glr.DepositID)
		if err != nil {
			if errors.Is(err, gemini.ErrInvalidCountry) {
				return handlers.WrapError(err, "region not supported", http.StatusBadRequest)
			}
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// LinkUpholdDepositAccountV3 - produces an http handler for the service s which handles deposit account linking of uphold wallets
func LinkUpholdDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			cuw = new(LinkUpholdDepositAccountRequest)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(ctx, cuw, r.Body); err != nil {
			return cuw.HandleErrors(err)
		}

		// get the wallet
		wallet, err := s.GetWallet(ctx, *id.UUID())
		if err != nil {
			if strings.Contains(err.Error(), "looking up wallet") {
				return handlers.WrapError(err, "unable to find wallet", http.StatusNotFound)
			}
			return handlers.WrapError(err, "unable to get or create wallets", http.StatusServiceUnavailable)
		}

		var aa uuid.UUID

		if cuw.AnonymousAddress != "" {
			aa, err = uuid.FromString(cuw.AnonymousAddress)
			if err != nil {
				return handlers.WrapError(err, "error parsing anonymous address", http.StatusBadRequest)
			}
		}

		publicKey, err := hex.DecodeString(wallet.PublicKey)
		if err != nil {
			logger.Warn().Err(err).Msg("unable to decode wallet public key")
			return handlers.WrapError(errors.New("unable to decode wallet public key"),
				"unable to decode wallet public key for creation request validation",
				http.StatusInternalServerError)
		}
		uwallet := uphold.Wallet{
			Info:    *wallet,
			PrivKey: ed25519.PrivateKey{},
			PubKey:  httpsignature.Ed25519PubKey([]byte(publicKey)),
		}

		err = s.LinkWallet(ctx, uwallet, cuw.SignedLinkingRequest, &aa)
		if err != nil {
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// GetWalletV3 - produces an http handler for the service s which handles getting of brave wallets
func GetWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var ctx = r.Context()
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	var id = new(inputs.ID)
	if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
		logger.Warn().Str("paymentId", err.Error()).Msg("failed to decode and validate paymentID from url")
		return handlers.ValidationError(
			"Error validating paymentID url parameter",
			map[string]interface{}{
				"paymentId": err.Error(),
			},
		)
	}

	var (
		roDB ReadOnlyDatastore
		ok   bool
	)

	// get datastore from context
	if roDB, ok = ctx.Value(appctx.RODatastoreCTXKey).(ReadOnlyDatastore); !ok {
		logger.Error().Msg("unable to get read only datastore from context")
	}

	// get wallet from datastore
	info, err := roDB.GetWallet(ctx, *id.UUID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Info().Err(err).Str("id", id.String()).Msg("wallet not found")
			return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
		}
		logger.Warn().Err(err).Str("id", id.String()).Msg("unable to get wallet")
		return handlers.WrapError(err, "error getting wallet from storage", http.StatusInternalServerError)
	}
	if info == nil {
		logger.Info().Err(err).Str("id", id.String()).Msg("wallet not found")
		return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
	}

	// render the wallet
	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusOK)
}

// RecoverWalletV3 - produces an http handler for the service s which handles recovering of brave wallets
func RecoverWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var ctx = r.Context()
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	var pk = new(inputs.PublicKey)
	if err := inputs.DecodeAndValidateString(ctx, pk, chi.URLParam(r, "publicKey")); err != nil {
		logger.Warn().Str("publicKey", err.Error()).Msg("failed to decode and validate publicKey from url")
		return handlers.ValidationError(
			"Error validating publicKey url parameter",
			map[string]interface{}{
				"publicKey": err.Error(),
			},
		)
	}

	var (
		roDB ReadOnlyDatastore
		ok   bool
	)

	// get datastore from context
	if roDB, ok = ctx.Value(appctx.RODatastoreCTXKey).(ReadOnlyDatastore); !ok {
		logger.Error().Msg("unable to get read only datastore from context")
	}

	// get wallet from datastore
	info, err := roDB.GetWalletByPublicKey(ctx, pk.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Info().Err(err).Str("pk", pk.String()).Msg("wallet not found")
			return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
		}
		logger.Warn().Err(err).Str("id", pk.String()).Msg("unable to get wallet")
		return handlers.WrapError(err, "error getting wallet from storage", http.StatusBadGateway)
	}
	if info == nil {
		logger.Info().Err(err).Str("id", pk.String()).Msg("wallet not found")
		return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
	}

	// render the wallet
	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusOK)
}

// GetUpholdWalletBalanceV3 - produces an http handler for the service s which handles balance inquiries of uphold wallets
func GetUpholdWalletBalanceV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var ctx = r.Context()
	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}
	// get the payment id from the URL request
	var id = new(inputs.ID)
	if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
		logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate payment id from url")
		return handlers.ValidationError(
			"Error validating paymentID url parameter",
			map[string]interface{}{
				"paymentID": err.Error(),
			},
		)
	}

	var (
		roDB ReadOnlyDatastore
		ok   bool
	)

	// get datastore from context
	if roDB, ok = ctx.Value(appctx.RODatastoreCTXKey).(ReadOnlyDatastore); !ok {
		logger.Error().Msg("unable to get read only datastore from context")
	}

	// get wallet from datastore
	info, err := roDB.GetWallet(ctx, *id.UUID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Info().Err(err).Str("id", id.String()).Msg("wallet not found")
			return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
		}
		logger.Warn().Err(err).Str("id", id.String()).Msg("unable to get wallet")
		return handlers.WrapError(err, "error getting wallet from storage", http.StatusInternalServerError)
	}
	if info == nil {
		logger.Info().Err(err).Str("id", id.String()).Msg("wallet not found")
		return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
	}

	if info.Provider != "uphold" {
		// not anoncard wallet, invalid
		logger.Warn().Str("id", id.String()).Msg("wallet not capable of balance inquiry")
		return handlers.WrapError(err, "wallet not capable of balance inquiry", http.StatusBadRequest)
	} else if info.ProviderID == "" { // implied only for uphold
		return handlers.WrapError(errors.New("provider id does not exist"), "wallet not capable of balance inquiry", http.StatusForbidden)
	}

	// convert this wallet to an uphold wallet
	uwallet := uphold.Wallet{
		Info: *info,
	}

	// get the wallet balance
	result, err := uwallet.GetBalance(ctx, true)
	if err != nil {
		logger.Info().Err(err).Str("id", id.String()).Msg("error getting balance from uphold")
		return handlers.WrapError(err, "failed to get balance from uphold", http.StatusInternalServerError)
	}

	// format the response and render
	return handlers.RenderContent(ctx, balanceToResponseV3(*result), w, http.StatusOK)
}

// LinkBraveDepositAccountV3 - produces an http handler for the service s which handles deposit account linking of brave wallets
func LinkBraveDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			lbw = new(LinkBraveDepositAccountRequest)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// validate payment id matches what was in the http signature
		signatureID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		if id.String() != signatureID {
			return handlers.ValidationError(
				"paymentId from URL does not match paymentId in http signature",
				map[string]interface{}{
					"paymentID": "does not match http signature id",
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(ctx, lbw, r.Body); err != nil {
			return lbw.HandleErrors(err)
		}

		linkedPaymentID, err := uuid.FromString(lbw.DepositDestination)
		if err != nil {
			return handlers.WrapError(err, "error parsing depositDestination", http.StatusBadRequest)
		}

		err = s.LinkBraveWallet(r.Context(), *id.UUID(), linkedPaymentID)
		if err != nil {
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// UnlinkWalletV3 - unlink a particular wallet from a custodian.
func UnlinkWalletV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx       = r.Context()
			walletID  = chi.URLParam(r, "payment_id")
			custodian = chi.URLParam(r, "custodian")
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		logger.Debug().
			Str("walletID", walletID).
			Str("custodian", custodian).
			Msg("unlinking wallet from custodian")
		err = s.UnlinkWallet(ctx, walletID, custodian)
		if err != nil {
			if errors.Is(err, ErrUnlinkingsExceeded) {
				logger.Warn().Err(err).Str("walletID", walletID).Msg("failed to unlink wallet")
				return handlers.WrapError(err, "error unlinking wallet", http.StatusForbidden)
			}
			logger.Error().Err(err).Str("walletID", walletID).Msg("failed to unlink wallet")
			return handlers.WrapError(err, "error unlinking wallet", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// IncreaseLinkingLimitV3 - increase the allowable linking limit for the specified paymentId by one
func IncreaseLinkingLimitV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx         = r.Context()
			custodianID = chi.URLParam(r, "custodian_id")
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		logger.Debug().Str("custodianId", custodianID).Msg("increasing linking limit for custodian id")
		err = s.IncreaseLinkingLimit(ctx, custodianID)
		if err != nil {
			logger.Error().Err(err).Str("custodianId", custodianID).Msg("failed to increase linking limit")
			return handlers.WrapError(err, "error increasing linking limit", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

// GetLinkingInfoV3 - get linking metadata
func GetLinkingInfoV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx       = r.Context()
			paymentID = new(inputs.ID)
			// default to a uuid that doesn't exist
			providerLinkingID = uuid.NewV4().String()
			custodianID       = r.URL.Query().Get("custodianId")
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		if r.URL.Query().Get("paymentId") != "" {
			// get payment id
			if err := inputs.DecodeAndValidateString(ctx, paymentID, r.URL.Query().Get("paymentId")); err != nil {
				logger.Warn().Err(err).Str("paymentID", r.URL.Query().Get("paymentId")).Msg("failed to decode and validate paymentID from url")
				return handlers.ValidationError(
					"error validating paymentID url parameter",
					map[string]interface{}{
						"paymentID": err.Error(),
					},
				)
			}
			// get the wallet
			wallet, err := s.GetWallet(ctx, *paymentID.UUID())
			if err != nil || wallet == nil {
				if wallet == nil || strings.Contains(err.Error(), "looking up wallet") {
					return handlers.WrapError(err, "unable to find wallet", http.StatusNotFound)
				}
				return handlers.WrapError(err, "unable to get linking limit for payment id", http.StatusServiceUnavailable)
			}
			if wallet.ProviderLinkingID != nil {
				providerLinkingID = wallet.ProviderLinkingID.String()
			}
		}

		info, err := s.GetLinkingInfo(ctx, providerLinkingID, custodianID)
		if err != nil {
			logger.Error().Err(err).Str("custodianId", custodianID).Msg("failed to get linking info")
			return handlers.WrapError(err, "error getting linking info", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, info, w, http.StatusOK)
	}
}

// DisconnectCustodianLinkV3 - produces an http handler for the service s which handles disconnect
// state for a deposit account linking
func DisconnectCustodianLinkV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx       = r.Context()
			id        = new(inputs.ID)
			custodian = new(CustodianName)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// get custodian name
		if err := inputs.DecodeAndValidateString(ctx, custodian, chi.URLParam(r, "custodian")); err != nil {
			logger.Warn().Str("custodian", err.Error()).Msg("failed to decode and validate custodian from url")
			return handlers.ValidationError(
				"error validating custodian url parameter",
				map[string]interface{}{
					"custodian": err.Error(),
				},
			)
		}

		sublogger := logger.With().
			Str("custodian", custodian.String()).
			Str("paymentID", id.String()).Logger()

		// validate payment id matches what was in the http signature
		signatureID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.ValidationError(
				"error validating http signature, does not match paymentID url parameter",
				map[string]interface{}{
					"signature": err.Error(),
				},
			)
		}

		if id.String() != signatureID {
			return handlers.ValidationError(
				"paymentId from URL does not match paymentId in http signature",
				map[string]interface{}{
					"paymentID": "does not match http signature id",
				},
			)
		}

		err = s.DisconnectCustodianLink(ctx, custodian.String(), *id.UUID())
		if err != nil {
			sublogger.Error().Err(err).Msg("failed to disconnect custodian link")
			return handlers.WrapError(err, "failed to disconnect custodian link", http.StatusInternalServerError)
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}
