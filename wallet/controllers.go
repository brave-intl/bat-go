package wallet

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for suggestions endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/{paymentId}/claim", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("LinkWalletCompat", LinkWalletCompat(service))))
	r.Method("GET", "/{paymentId}", middleware.InstrumentHandler("GetWallet", GetWallet(service)))
	r.Method("POST", "/", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostCreateWallet", PostCreateWallet(service))))
	return r
}

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.GetOrCreateWallet(ctx, walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

// LinkWalletRequest holds the data necessary to update a wallet with an anonymous address
type LinkWalletRequest struct {
	SignedTx         string     `json:"signedTx"`
	AnonymousAddress *uuid.UUID `json:"anonymousAddress"`
}

// LinkWalletCompat links wallets using provided ids
func LinkWalletCompat(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		paymentIDString := chi.URLParam(r, "paymentID")
		paymentID, err := uuid.FromString(paymentIDString)
		if err != nil {
			return handlers.ValidationError("url parameter", map[string]string{
				"paymentID": "must be a valid uuidv4",
			})
		}

		var body LinkWalletRequest
		err = requestutils.ReadJSON(r.Body, &body)
		if err != nil {
			return handlers.ValidationError("request body", map[string]string{
				"body": "unable to ready body",
			})
		}
		_, err = govalidator.ValidateStruct(body)
		if err != nil {
			return handlers.WrapValidationError(err)
		}
		// remove this check and merge when ledger endpoint is depricated
		wallet, err := service.GetAndCreateMemberWallets(r.Context(), paymentID)
		if err != nil {
			if err == errorutils.ErrWalletNotFound {
				return handlers.WrapError(err, "unable to find wallet", http.StatusNotFound)
			}
			return handlers.WrapError(err, "unable to backfill wallets", http.StatusServiceUnavailable)
		}
		err = service.LinkWallet(r.Context(), wallet, body.SignedTx, body.AnonymousAddress)
		if err != nil {
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		return handlers.RenderContent(r.Context(), wallet, w, http.StatusOK)
	})
}

// GetWalletResponse gets wallet info tied to a wallet id
type GetWalletResponse struct {
	Wallet *walletutils.Info `json:"wallet"`
}

// PostCreateWalletRequest has possible inputs for the new wallet
type PostCreateWalletRequest struct {
	Provider string `json:"provider" valid:"in(brave|uphold)"`
	SignedTx string `json:"signedTx" valid:"-"`
}

func validateHTTPSignature(ctx context.Context, r *http.Request, signature string) (string, error) {
	// validate that the signature in the header is valid based on public key provided
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(signature))
	if err != nil {
		return "", errors.New("invalid signature")
	}

	// Override algorithm and headers to those we want to enforce
	s.Algorithm = httpsignature.ED25519
	s.Headers = []string{"digest", "(request-target)"}
	var publicKey httpsignature.Ed25519PubKey
	if len(s.KeyID) > 0 {
		var err error
		publicKey, err = hex.DecodeString(s.KeyID)
		if err != nil {
			return "", fmt.Errorf("failed to hex decode public key: %w", err)
		}
	}
	pubKey := httpsignature.Verifier(publicKey)
	if err != nil {
		return "", err
	}
	if pubKey == nil {
		return "", errors.New("invalid public key")
	}

	valid, err := s.Verify(pubKey, crypto.Hash(0), r)

	if err != nil {
		return "", errors.New("failed to verify signature")
	}
	if !valid {
		return "", errors.New("invalid signature")
	}
	return s.KeyID, nil
}

// PostCreateWallet creates a wallet
func PostCreateWallet(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		publicKey, err := validateHTTPSignature(r.Context(), r, r.Header.Get("Signature"))
		if err != nil {
			return handlers.WrapError(err, "invalid http signature", http.StatusForbidden)
		}
		var req PostCreateWalletRequest
		err = requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// no more uphold wallets in the wild please
		if req.Provider == "uphold" && os.Getenv("ENV") != "local" {
			return handlers.WrapError(errors.New("unable to create uphold wallet"), "failed to create wallet", http.StatusBadRequest)
		}

		info, err := CreateWallet(req, publicKey)
		if err != nil {
			return handlers.WrapError(err, "unable to save wallet", http.StatusServiceUnavailable)
		}
		err = service.Datastore.InsertWallet(&info)
		if err != nil {
			return handlers.WrapError(err, "unable to save wallet", http.StatusServiceUnavailable)
		}

		return handlers.RenderContent(r.Context(), info, w, http.StatusCreated)
	})
}

// GetWallet retrieves wallet information
func GetWallet(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		paymentIDParam := chi.URLParam(r, "paymentId")
		paymentID, err := uuid.FromString(paymentIDParam)

		if err != nil {
			return handlers.ValidationError("request url parameter", map[string]string{
				"paymentId": "paymentId '" + paymentIDParam + "' is not supported",
			})
		}

		info, err := service.Datastore.GetWallet(paymentID)
		if err != nil {
			return handlers.WrapError(err, "Error getting wallet", http.StatusNotFound)
		}

		// just doing this until another way to track
		if info.AltCurrency == nil {
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		return handlers.RenderContent(r.Context(), info, w, http.StatusOK)
	})
}

// CreateWallet creates a new set of wallet info
func CreateWallet(req PostCreateWalletRequest, publicKey string) (walletutils.Info, error) {
	provider := req.Provider // client

	var info walletutils.Info
	info.ID = uuid.NewV4().String()
	info.Provider = provider
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	info.PublicKey = publicKey

	if req.Provider == "uphold" {
		if req.SignedTx != "" {
			wallet := uphold.Wallet{
				Info:    info,
				PrivKey: ed25519.PrivateKey{},
				PubKey:  httpsignature.Ed25519PubKey([]byte(publicKey)),
			}
			err := wallet.SubmitRegistration(req.SignedTx)
			if err != nil {
				return info, err
			}
			info.ProviderID = wallet.GetWalletInfo().ProviderID
		}
	}
	return info, nil
}

// ------------------ V3 below ---------------

// CreateUpholdWalletV3 - produces an http handler for the service s which handles creation of uphold wallets
func CreateUpholdWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	publicKey, err := validateHTTPSignature(r.Context(), r, r.Header.Get("Signature"))
	if err != nil {
		return handlers.WrapError(err, "invalid http signature", http.StatusForbidden)
	}

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

	// no more uphold wallets in the wild please
	if env, ok := r.Context().Value(appctx.EnvironmentCTXKey).(string); ok && env == "local" {
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

	if ucReq.SignedCreationRequest != "" {
		uwallet := uphold.Wallet{
			Info:    *info,
			PrivKey: ed25519.PrivateKey{},
			PubKey:  httpsignature.Ed25519PubKey([]byte(publicKey)),
		}
		if err := uwallet.SubmitRegistration(ucReq.SignedCreationRequest); err != nil {
			return handlers.WrapError(
				errors.New("unable to create uphold wallet"),
				"failed to register wallet with uphold", http.StatusServiceUnavailable)
		}
		info.ProviderID = uwallet.GetWalletInfo().ProviderID
	}

	// get wallet from datastore
	err = db.InsertWallet(info)
	if err != nil {
		logger.Error().Err(err).Str("id", info.ID).Msg("unable to create brave wallet")
		return handlers.WrapError(err, "error writing wallet to storage", http.StatusServiceUnavailable)
	}

	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusOK)
}

// CreateBraveWalletV3 - produces an http handler for the service s which handles creation of brave wallets
func CreateBraveWalletV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	// perform validation based on public key that the user submits
	publicKey, err := validateHTTPSignature(r.Context(), r, r.Header.Get("Signature"))
	if err != nil {
		return handlers.WrapError(err, "invalid http signature", http.StatusForbidden)
	}

	var (
		ctx = r.Context()
		bcr = new(BraveCreationRequest)
	)

	// get logger from context
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		// no logger, setup
		ctx, logger = logging.SetupLogger(ctx)
	}

	if err := inputs.DecodeAndValidateReader(r.Context(), bcr, r.Body); err != nil {
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
		PublicKey:   publicKey,
		AltCurrency: &altCurrency,
	}

	// get wallet from datastore
	err = db.InsertWallet(info)
	if err != nil {
		logger.Error().Err(err).Str("id", info.ID).Msg("unable to create brave wallet")
		return handlers.WrapError(err, "error writing wallet to storage", http.StatusServiceUnavailable)
	}

	return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusOK)
}

// ClaimUpholdWalletV3 - produces an http handler for the service s which handles claiming of uphold wallets
func ClaimUpholdWalletV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var (
			ctx = r.Context()
			id  = new(inputs.ID)
			cuw = new(ClaimUpholdWalletRequest)
		)
		// get logger from context
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
		}

		// get payment id
		if err := inputs.DecodeAndValidateString(context.Background(), id, chi.URLParam(r, "paymentID")); err != nil {
			logger.Warn().Str("paymentID", err.Error()).Msg("failed to decode and validate paymentID from url")
			return handlers.ValidationError(
				"error validating paymentID url parameter",
				map[string]interface{}{
					"paymentID": err.Error(),
				},
			)
		}

		// read post body
		if err := inputs.DecodeAndValidateReader(r.Context(), cuw, r.Body); err != nil {
			return cuw.HandleErrors(err)
		}

		// remove this check and merge when ledger endpoint is depricated
		wallet, err := s.GetAndCreateMemberWallets(ctx, id.UUID())
		if err != nil {
			if err == errorutils.ErrWalletNotFound {
				return handlers.WrapError(err, "unable to find wallet", http.StatusNotFound)
			}
			return handlers.WrapError(err, "unable to backfill wallets", http.StatusServiceUnavailable)
		}
		aa, err := uuid.FromString(cuw.AnonymousAddress)
		if err != nil {
			return handlers.ValidationError(
				"error validating anonymousAddress",
				map[string]interface{}{
					"anonymousAddress": err.Error(),
				},
			)
		}

		err = s.LinkWallet(r.Context(), wallet, cuw.SignedCreationRequest, &aa)
		if err != nil {
			return handlers.WrapError(err, "error linking wallet", http.StatusBadRequest)
		}

		// render the wallet
		return handlers.RenderContent(ctx, infoToResponseV3(wallet), w, http.StatusOK)
	}
}

// ClaimBraveWalletV3 - produces an http handler for the service s which handles claiming of brave wallets
func ClaimBraveWalletV3(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handlers.RenderContent(r.Context(), "not implemented", w, http.StatusNotImplemented)
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
	if err := inputs.DecodeAndValidateString(context.Background(), id, chi.URLParam(r, "paymentID")); err != nil {
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
	info, err := roDB.GetWallet(id.UUID())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Info().Err(err).Str("id", id.String()).Msg("wallet not found")
			return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
		}
		logger.Warn().Err(err).Str("id", id.String()).Msg("unable to get wallet")
		return handlers.WrapError(err, "error getting wallet from storage", http.StatusBadGateway)
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
	if err := inputs.DecodeAndValidateString(context.Background(), pk, chi.URLParam(r, "publicKey")); err != nil {
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
	info, err := roDB.GetWalletByPublicKey(pk.String())
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
	return handlers.RenderContent(r.Context(), "not implemented", w, http.StatusNotImplemented)
}

// GetBraveWalletBalance - produces an http handler for the service s which handles balance inquiries of brave wallets
func GetBraveWalletBalanceV3(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return handlers.RenderContent(r.Context(), "not implemented", w, http.StatusNotImplemented)
}
