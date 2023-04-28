package payments

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/middleware"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	"github.com/brave-intl/bat-go/libs/requestutils"
)

type configurationHandlerRequest map[appctx.CTXKey]interface{}

type getConfResponse struct {
	AttestationDocument string `json:"attestation"`
	PublicKey           string
}

// GetConfigurationHandler - handler to get important payments configuration information, attested by nitro
func GetConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger := logging.Logger(ctx, "GetConfigurationHandler")
		nonce := make([]byte, 64)
		_, err := rand.Read(nonce)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create random nonce")
			return handlers.WrapError(err, "failed to create random nonce", http.StatusBadRequest)
		}

		attestationDocument, err := nitro.Attest(nonce, []byte{}, []byte{})
		if err != nil {
			logger.Error().Err(err).Msg("failed to get attestation from nitro")
			return handlers.WrapError(err, "failed to get attestation from nitro", http.StatusBadRequest)
		}

		resp := &getConfResponse{
			// return the attestation document
			AttestationDocument: base64.StdEncoding.EncodeToString(attestationDocument),
		}

		logger.Debug().Msg("handling configuration request")
		// return ok, at this point all new requests will use the new baseCtx of the service
		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}

// PrepareHandler - handler to get current relative exchange rates
func PrepareHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PrepareHandler")
			req    = new(Transaction)
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		logger.Debug().Str("request", fmt.Sprintf("%+v", req)).Msg("structure of request")
		// validate the transaction

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", req)).Msg("failed to validate structure")
			return handlers.WrapError(err, "failed to validate transaction", http.StatusBadRequest)
		}

		// sign the transaction
		req.PublicKey, req.Signature, err = req.SignTransaction(ctx, service.kmsSigningClient, service.kmsSigningKeyId)
		if err != nil {
			logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", req)).Msg("failed to sign transaction")
			return handlers.WrapError(err, "failed to sign transaction", http.StatusBadRequest)
		}

		logger.Debug().Str("transaction", fmt.Sprintf("%+v", req)).Msg("handling prepare request")

		// returns an enriched list of transactions, which includes the document metadata
		resp, err := service.InsertTransaction(ctx, req)
		if err != nil {
			return handlers.WrapError(err, "failed to insert transactions", http.StatusInternalServerError)
		}

		// create a random nonce for nitro attestation
		nonce := make([]byte, 64)
		_, err = rand.Read(nonce)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create random nonce")
			return handlers.WrapError(err, "failed to create random nonce", http.StatusBadRequest)
		}

		// attest over the transaction
		attestationDocument, err := nitro.Attest(nonce, resp.BuildSigningBytes(), []byte{})
		if err != nil {
			logger.Error().Err(err).Msg("failed to get attestation from nitro")
			return handlers.WrapError(err, "failed to get attestation from nitro", http.StatusBadRequest)
		}

		resp.AttestationDocument = base64.StdEncoding.EncodeToString(attestationDocument)
		// Should be in QLDB in prepared state

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}

// SubmitHandler - handler to perform submission of transactions to custodian
func SubmitHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "SubmitHandler")
			req    = &Transaction{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", req)).Msg("failed to validate structure")
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", req)).Msg("handling submit request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// attempt authorization on the transaction
		err = service.AuthorizeTransaction(ctx, keyID, *req)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		// TODO: check if business logic was met from authorizers table in qldb for this transaction
		// TODO: state machine handling for custodian submissions

		// get the current state of the transaction from qldb
		resp, err := service.GetTransactionFromDocID(ctx, req.DocumentID)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		// create a random nonce for nitro attestation
		nonce := make([]byte, 64)
		_, err = rand.Read(nonce)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create random nonce")
			return handlers.WrapError(err, "failed to create random nonce", http.StatusBadRequest)
		}

		// attest over the transaction
		attestationDocument, err := nitro.Attest(nonce, resp.BuildSigningBytes(), []byte{})
		if err != nil {
			logger.Error().Err(err).Msg("failed to get attestation from nitro")
			return handlers.WrapError(err, "failed to get attestation from nitro", http.StatusBadRequest)
		}

		resp.AttestationDocument = base64.StdEncoding.EncodeToString(attestationDocument)

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}
