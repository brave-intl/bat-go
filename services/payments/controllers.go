package payments

import (
	"crypto"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/middleware"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/nitro"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
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
			req    = new(paymentLib.PrepareRequest)
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

		authenticatedState := req.ToAuthenticatedPaymentState()

		// Ensure that prepare succeeds ( i.e. we are not using a failing dry-run state machine )
		stateMachine, err := service.StateMachineFromTransaction(authenticatedState)
		if err != nil {
			return handlers.WrapError(err, "failed to create stateMachine", http.StatusBadRequest)
		}
		_, err = stateMachine.Prepare(ctx)
		if err != nil {
			return handlers.WrapError(err, "could not put transaction into the prepared state", http.StatusBadRequest)
		}

		paymentState, err := authenticatedState.ToPaymentState()
		if err != nil {
			return handlers.WrapError(err, "could not create a payment state", http.StatusBadRequest)
		}

		err = paymentState.Sign(service.signer, service.publicKey)
		if err != nil {
			return handlers.WrapError(err, "failed to sign payment state", http.StatusInternalServerError)
		}

		documentID, err := service.datastore.InsertPaymentState(ctx, paymentState)
		if err != nil {
			return handlers.WrapError(err, "failed to insert payment state", http.StatusInternalServerError)
		}
		resp := paymentLib.PrepareResponse{
			PaymentDetails: req.PaymentDetails,
			DocumentID:     documentID,
		}

		logger.Debug().Str("transaction", fmt.Sprintf("%+v", req)).Msg("handling prepare request")

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
			submitRequest  = &paymentLib.SubmitRequest{}
			submitResponse = paymentLib.SubmitResponse{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &submitRequest)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(submitRequest)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", submitRequest)).Msg("handling submit request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapError(err, "error getting identity of transaction authorizer", http.StatusInternalServerError)
		}

		// get the history of the transaction from qldb
		history, err := service.datastore.GetPaymentStateHistory(ctx, submitRequest.DocumentID)
		if err != nil {
			return handlers.WrapError(err, "failed to get history from document id", http.StatusInternalServerError)
		}

		// validate the history of the transaction
		authenticatedState, err := history.GetAuthenticatedPaymentState(service.verifier, submitRequest.DocumentID)
		if err != nil {
			return handlers.WrapError(err, "failed to validate payment state history", http.StatusInternalServerError)
		}

		// attempt authorization on the transaction
		err = service.AuthorizeTransaction(ctx, keyID, authenticatedState)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		err = service.DriveTransaction(ctx, authenticatedState)
		if err != nil {
			// TODO: if error is permanent, return 200
			return handlers.WrapError(err, "failed to drive transaction", http.StatusInternalServerError)
		}

		submitResponse.Status = authenticatedState.Status

		// NOTE: we are intentionally returning an AppError even in the success case as some errors are
		// "permanent" errors indiciating a transaction state machine has reached an end state
		return &handlers.AppError{
			Cause:   nil,
			Message: "submit succeeded",
			Code:    http.StatusOK,
			Data:    submitResponse,
		}
	}
}
