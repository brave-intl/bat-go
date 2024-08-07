package wallet

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/libs/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/services/wallet/model"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

const (
	reqBodyLimit10MB = 10 << 20
)

// LinkDepositAccountResponse is the response returned by the linking endpoints.
type LinkDepositAccountResponse struct {
	GeoCountry string `json:"geoCountry"`
}

// CreateUpholdWalletV3 produces a http handler for the service which handles creation of uphold wallets.
func CreateUpholdWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var (
		ucReq       = new(UpholdCreationRequest)
		ctx         = r.Context()
		altCurrency = altcurrency.BAT
	)

	logger := logging.Logger(ctx, "wallet.CreateUpholdWalletV3")

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
		return handlers.WrapError(errors.New("unable to get datastore"), "misconfigured datastore", http.StatusServiceUnavailable)
	}

	var info = &walletutils.Info{
		ID:          uuid.NewV4().String(),
		Provider:    "uphold",
		PublicKey:   publicKey,
		AltCurrency: &altCurrency,
	}

	uwallet := uphold.Wallet{
		Info:    *info,
		PrivKey: httpsignature.Ed25519PrivKey{},
		PubKey:  httpsignature.Ed25519PubKey(publicKey),
	}
	if err := uwallet.SubmitRegistration(ctx, ucReq.SignedCreationRequest); err != nil {
		return handlers.WrapError(
			errors.New("unable to create uphold wallet"),
			"failed to register wallet with uphold", http.StatusServiceUnavailable)
	}
	info.ProviderID = uwallet.GetWalletInfo().ProviderID

	// get wallet from datastore
	err := db.InsertWallet(ctx, info)
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
	logger := logging.Logger(ctx, "wallet.CreateBraveWalletV3")

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

		l := logging.Logger(ctx, "wallet.CreateBitflyerWalletV3")

		// check if we have disabled bitflyer
		if disableBitflyer, ok := ctx.Value(appctx.DisableBitflyerLinkingCTXKey).(bool); ok && disableBitflyer {
			return handlers.ValidationError(
				"Connecting Brave Rewards to Bitflyer is temporarily unavailable.  Please try again later",
				nil,
			)
		}

		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			l.Warn().Err(err).Msg("failed to decode and validate paymentID from url")
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

		country, err := s.LinkBitFlyerWallet(ctx, *id.UUID(), blr.DepositID, blr.AccountHash)
		if err != nil {
			l.Error().Err(err).Str("paymentID", id.String()).Msg("failed to link bitflyer wallet")
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, LinkDepositAccountResponse{
			GeoCountry: country,
		}, w, http.StatusOK)
	}
}

// LinkZebPayDepositAccountV3 returns a handler which handles deposit account linking of zebpay wallets.
func LinkZebPayDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		// Check whether it's disabled.
		if disable, ok := ctx.Value(appctx.DisableZebPayLinkingCTXKey).(bool); ok && disable {
			const msg = "Connecting Brave Rewards to ZebPay is temporarily unavailable. Please try again later"
			return handlers.ValidationError(msg, nil)
		}

		id := &inputs.ID{}
		l := logging.Logger(ctx, "wallet.LinkZebPayDepositAccountV3")

		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			l.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")

			const msg = "error validating paymentID url parameter"
			return handlers.ValidationError(msg, map[string]interface{}{"paymentID": err.Error()})
		}

		// Check that payment id matches what was in the http signature.
		signatureID, err := middleware.GetKeyID(ctx)
		if err != nil {
			const msg = "error validating paymentID url parameter"
			return handlers.ValidationError(msg, map[string]interface{}{"paymentID": err.Error()})
		}

		if id.String() != signatureID {
			const msg = "paymentId from URL does not match paymentId in http signature"
			return handlers.ValidationError(msg, map[string]interface{}{
				"paymentID": "does not match http signature id",
			})
		}

		zplReq := &ZebPayLinkingRequest{}
		if err := inputs.DecodeAndValidateReader(ctx, zplReq, r.Body); err != nil {
			return HandleErrorsZebPay(err)
		}

		country, err := s.LinkZebPayWallet(ctx, *id.UUID(), zplReq.VerificationToken)
		if err != nil {
			l.Error().Err(err).Str("paymentID", id.String()).Msg("failed to link wallet")
			switch {
			case errors.Is(err, errorutils.ErrInvalidCountry):
				return handlers.WrapError(err, "region not supported", http.StatusBadRequest)
			case errors.Is(err, errZPInvalidKYC):
				return handlers.WrapError(err, "KYC required", http.StatusForbidden)
			default:
				return handlers.WrapError(err, err.Error(), http.StatusBadRequest)
			}
		}

		return handlers.RenderContent(ctx, LinkDepositAccountResponse{
			GeoCountry: country,
		}, w, http.StatusOK)
	}
}

// LinkGeminiDepositAccountV3 returns an HTTP handler which is responsible for linking a Gemini wallet.
// This endpoint expects a walletID as part of the URL and takes a verification token which encodes the
// linking information as well as a recipientID. The recipientID is synonymous with a wallets depositID.
func LinkGeminiDepositAccountV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			glr = new(GeminiLinkingRequest)
		)

		l := logging.Logger(ctx, "wallet.LinkGeminiDepositAccountV3")

		// check if we have disabled Gemini
		if disableGemini, ok := ctx.Value(appctx.DisableGeminiLinkingCTXKey).(bool); ok && disableGemini {
			return handlers.ValidationError(
				"Connecting Brave Rewards to Gemini is temporarily unavailable.  Please try again later",
				nil,
			)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			l.Warn().Str("paymentID", id.String()).Err(err).
				Msg("failed to decode and validate paymentID from url")
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
			l.Warn().Str("paymentID", id.String()).
				Err(err).Msg("could not get http signing key id from context")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		if id.String() != signatureID {
			l.Warn().Str("paymentID", id.String()).
				Msg("id does not match signature id")
			return handlers.ValidationError(
				"paymentId from URL does not match paymentId in http signature",
				map[string]interface{}{
					"paymentID": "does not match http signature id",
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(ctx, glr, r.Body); err != nil {
			l.Warn().Str("paymentID", id.String()).
				Err(err).Msg("could not validate request")
			return glr.HandleErrors(err)
		}

		country, err := s.LinkGeminiWallet(ctx, *id.UUID(), glr.VerificationToken, glr.DepositID)
		if err != nil {
			l.Error().Str("paymentID", id.String()).
				Err(err).Msg("error linking gemini wallet")

			if errors.Is(err, errorutils.ErrInvalidCountry) {
				return handlers.WrapError(err, "region not supported", http.StatusBadRequest)
			}

			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		return handlers.RenderContent(ctx, LinkDepositAccountResponse{
			GeoCountry: country,
		}, w, http.StatusOK)
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
		l := logging.Logger(ctx, "wallet.LinkUpholdDepositAccountV3")

		// check if we have disabled uphold
		if disableUphold, ok := ctx.Value(appctx.DisableUpholdLinkingCTXKey).(bool); ok && disableUphold {
			return handlers.ValidationError(
				"Connecting Brave Rewards to Uphold is temporarily unavailable.  Please try again later",
				nil,
			)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
			l.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
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
			l.Warn().Err(err).Msg("unable to decode wallet public key")
			return handlers.WrapError(errors.New("unable to decode wallet public key"),
				"unable to decode wallet public key for creation request validation",
				http.StatusInternalServerError)
		}
		uwallet := uphold.Wallet{
			Info:    *wallet,
			PrivKey: httpsignature.Ed25519PrivKey{},
			PubKey:  httpsignature.Ed25519PubKey(publicKey),
		}

		country, err := s.LinkUpholdWallet(ctx, uwallet, cuw.SignedLinkingRequest, &aa)
		if err != nil {
			l.Error().Err(err).Str("paymentID", id.String()).
				Msg("failed to link wallet")
			if errors.Is(err, errorutils.ErrInvalidCountry) {
				return handlers.WrapError(err, "region not supported", http.StatusBadRequest)
			}
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, LinkDepositAccountResponse{
			GeoCountry: country,
		}, w, http.StatusOK)
	}
}

const errOriginForbidden model.Error = "request origin forbidden"

type linkSolanaAddrRequest struct {
	SolanaPublicKey string `json:"solanaPublicKey" valid:"length(32|44)"`
	Message         string `json:"message" valid:"required"`
	SolanaSignature string `json:"solanaSignature" valid:"required"`
}

func LinkSolanaAddress(s *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()

		if dis, ok := ctx.Value(appctx.DisableSolanaLinkingCTXKey).(bool); ok && dis {
			return handlers.ValidationError("Connecting Brave Rewards to Solana is temporarily unavailable. Please try again later", nil)
		}

		l := logging.Logger(ctx, "wallet")

		o := r.Header.Get("Origin")
		if !isAllowedOrigin(o, s.dappConf.AllowedOrigins) {
			l.Error().Err(errOriginForbidden).Str("origin", strOr(o, "empty")).Msg("error linking solana address")
			return handlers.WrapError(errOriginForbidden, "request origin forbidden", http.StatusForbidden)
		}

		var paymentID inputs.ID
		if err := inputs.DecodeAndValidateString(ctx, &paymentID, chi.URLParam(r, "paymentID")); err != nil {
			return handlers.WrapError(err, "invalid paymentID", http.StatusBadRequest)
		}

		b, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			return handlers.WrapError(err, "error reading body", http.StatusBadRequest)
		}

		var solReq linkSolanaAddrRequest
		if err := json.Unmarshal(b, &solReq); err != nil {
			return handlers.WrapError(err, "error decoding body", http.StatusBadRequest)
		}

		if _, err := govalidator.ValidateStruct(solReq); err != nil {
			return handlers.WrapValidationError(err)
		}

		if err := s.LinkSolanaAddress(ctx, *paymentID.UUID(), solReq); err != nil {
			l.Error().Err(err).Msg("error linking solana address")

			var solErr *walletutils.LinkSolanaAddressError
			switch {
			case errors.Is(err, model.ErrWalletNotWhitelisted):
				return handlers.WrapError(model.ErrWalletNotWhitelisted, "rewards wallet not whitelisted", http.StatusForbidden)
			case errors.Is(err, model.ErrChallengeNotFound):
				return handlers.WrapError(model.ErrChallengeNotFound, "linking challenge not found", http.StatusNotFound)
			case errors.Is(err, model.ErrChallengeExpired):
				return handlers.WrapError(model.ErrChallengeExpired, "linking challenge expired", http.StatusUnauthorized)
			case errors.Is(err, model.ErrWalletNotFound):
				return handlers.WrapError(model.ErrWalletNotFound, "rewards wallet not found", http.StatusNotFound)
			case errors.Is(err, errDisabledRegion):
				return handlers.WrapError(errDisabledRegion, "region is currently disabled for linking", http.StatusBadRequest)
			case errors.Is(err, ErrTooManyCardsLinked):
				return handlers.WrapError(ErrTooManyCardsLinked, "too many wallets linked", http.StatusConflict)
			case errors.As(err, &solErr):
				return handlers.WrapError(solErr, "invalid solana linking message", http.StatusUnauthorized)
			default:
				return handlers.WrapError(model.ErrInternalServer, "internal server error", http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, nil, w, http.StatusOK)
	}
}

type challengeRequest struct {
	PaymentID uuid.UUID `json:"paymentId"`
}

type challengeResponse struct {
	Nonce string `json:"challengeId"`
}

func CreateChallenge(s *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		b, err := io.ReadAll(io.LimitReader(r.Body, reqBodyLimit10MB))
		if err != nil {
			return handlers.WrapError(err, "error reading body", http.StatusBadRequest)
		}

		var chlReq challengeRequest
		if err := json.Unmarshal(b, &chlReq); err != nil {
			return handlers.WrapError(err, "error decoding body", http.StatusBadRequest)
		}

		if uuid.Equal(chlReq.PaymentID, uuid.Nil) {
			return handlers.ValidationError("request", map[string]interface{}{
				"paymentID": "cannot be nil or empty",
			})
		}

		ctx := r.Context()

		chl, err := s.CreateChallenge(ctx, chlReq.PaymentID)
		if err != nil {
			logging.Logger(ctx, "wallet").Error().Err(err).Msg("error creating challenge")
			return handlers.WrapError(model.ErrInternalServer, "error creating challenge", http.StatusInternalServerError)
		}

		resp := challengeResponse{
			Nonce: chl.Nonce,
		}

		return handlers.RenderContent(ctx, resp, w, http.StatusCreated)
	}
}

// GetWalletV3 returns a rewards wallet for the given paymentID.
func GetWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var ctx = r.Context()

	l := logging.Logger(ctx, "wallet.GetWalletV3")

	var id = new(inputs.ID)
	if err := inputs.DecodeAndValidateString(ctx, id, chi.URLParam(r, "paymentID")); err != nil {
		l.Warn().Err(err).Str("paymentID", id.String()).Msg("failed to decode and validate paymentID from url")
		return handlers.ValidationError("Error validating paymentID url parameter", map[string]interface{}{
			"paymentId": err.Error(),
		})
	}

	// TODO(clD11): this should be removed from ctx as part of wallet refactor. Note, the service would have already
	//  panicked at startup if the db is missing. However, as a precaution we should stop processing.
	roDB, ok := ctx.Value(appctx.RODatastoreCTXKey).(ReadOnlyDatastore)
	if !ok {
		return handlers.WrapError(errorutils.ErrInternalServerError, "db missing from context", http.StatusInternalServerError)
	}

	info, err := roDB.GetWallet(ctx, *id.UUID())
	if err != nil {
		l.Error().Err(err).Str("paymentID", id.String()).Msg("error getting wallet")
		return handlers.WrapError(err, "error getting wallet from storage", http.StatusInternalServerError)
	}

	if info == nil {
		l.Info().Str("paymentID", id.String()).Msg("wallet not found")
		return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
	}

	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusOK)
}

// RecoverWalletV3 - produces an http handler for the service s which handles recovering of brave wallets
func RecoverWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	var ctx = r.Context()
	// get logger from context
	logger := logging.Logger(ctx, "wallet.RecoverWalletV3")

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
	logger := logging.Logger(ctx, "wallet.GetUpholdWalletBalanceV3")
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
		logger := logging.Logger(ctx, "wallet.GetLinkingInfoV3")

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
		logger := logging.Logger(ctx, "wallet.DisconnectCustodianLinkV3")

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

		return handlers.RenderContent(ctx, map[string]interface{}{}, w, http.StatusOK)
	}
}

func isAllowedOrigin(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return false
	}

	for i := range allowedOrigins {
		if allowedOrigins[i] == origin {
			return true
		}
	}

	return false
}

func strOr(a string, b string) string {
	if a == "" {
		return b
	}
	return a
}
