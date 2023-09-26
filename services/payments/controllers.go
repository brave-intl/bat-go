package payments

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"crypto"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/middleware"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	. "github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/requestutils"
)

type getConfResponse struct {
	AttestationDocument string `json:"attestation"`
	PublicKey           string
}

// GetConfigurationHandler gets important payments configuration information, attested by nitro.
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

		attestationDocument, err := nitro.Attest(ctx, nonce, []byte{}, []byte{})
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

// PrepareHandler attempts to create a new record in QLDB for the transaction. When it completes
// successfully, the record is in the Prepared state. If the record already exists, preparation
// will fail..
func PrepareHandler(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		nitroKey := httpsignature.NitroSigner{}

		var sp httpsignature.SignatureParams
		sp.Algorithm = httpsignature.AWSNITRO
		sp.KeyID = "primary"
		sp.Headers = []string{"digest"}

		ps := httpsignature.ParameterizedSignator{
			SignatureParams: sp,
			Signator:        nitroKey,
			Opts:            crypto.Hash(0),
		}
		w = httpsignature.NewParameterizedSignatorResponseWriter(ps, w)

		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PrepareHandler")
			req    = new(PrepareRequest)
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

		// returns an enriched list of transactions, which includes the document metadata

		documentID, err := service.insertPayment(ctx, req.PaymentDetails)
		if err != nil {
			return handlers.WrapError(err, "failed to insert payment", http.StatusInternalServerError)
		}
		resp := PrepareResponse{
			PaymentDetails: req.PaymentDetails,
			DocumentID:     documentID,
		}

		logger.Debug().Str("transaction", fmt.Sprintf("%+v", req)).Msg("handling prepare request")

		// create a random nonce for nitro attestation
		nonce := make([]byte, 64)
		_, err = rand.Read(nonce)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create random nonce")
			return handlers.WrapError(err, "failed to create random nonce", http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	}
}

// SubmitHandler performs submission of transactions to custodian.
func SubmitHandler(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		nitroKey := httpsignature.NitroSigner{}

		var sp httpsignature.SignatureParams
		sp.Algorithm = httpsignature.AWSNITRO
		sp.KeyID = "primary"
		sp.Headers = []string{"digest"}

		ps := httpsignature.ParameterizedSignator{
			SignatureParams: sp,
			Signator:        nitroKey,
			Opts:            crypto.Hash(0),
		}
		w = httpsignature.NewParameterizedSignatorResponseWriter(ps, w)
		// get context from request
		ctx := r.Context()

		var (
			logger         = logging.Logger(ctx, "SubmitHandler")
			submitRequest  = &SubmitRequest{}
			submitResponse = SubmitResponse{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &submitRequest)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(submitRequest)
		if err != nil {
			logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", submitRequest)).Msg("failed to validate structure")
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", submitRequest)).Msg("handling submit request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		// get the current state of the transaction from qldb
		authenticatedState, _, err := service.GetTransactionFromDocumentID(ctx, submitRequest.DocumentID)
		if err != nil {
			//return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
			submitResponse.LastError = &PaymentError{
				OriginalError: err,
			}
			return handlers.RenderContent(r.Context(), submitResponse, w, http.StatusOK)
		}

		// attempt authorization on the transaction
		err = service.AuthorizeTransaction(ctx, keyID, *authenticatedState)
		if err != nil {
			//return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
			submitResponse.LastError = &PaymentError{
				OriginalError: err,
			}
			return handlers.RenderContent(r.Context(), submitResponse, w, http.StatusOK)
		}

		submitResponse.Status = authenticatedState.Status

		// TODO: check if business logic was met from authorizers table in qldb for this transaction
		// TODO: state machine handling for custodian submissions

		return handlers.RenderContent(r.Context(), submitResponse, w, http.StatusOK)
	}
}
