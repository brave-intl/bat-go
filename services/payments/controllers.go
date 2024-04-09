package payments

import (
	"context"
	"crypto"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/rs/zerolog/hlog"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/libs/nitro"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/requestutils"
)

type getConfResponse struct {
	EncryptionKeyARN string `json:"encryptionKeyArn"`
	Environment      string `json:"environment"`
}

func SetupRouter(ctx context.Context, s *Service) (context.Context, *chi.Mux) {
	// base service logger
	logger := logging.Logger(ctx, "payments")
	// base router
	r := chi.NewRouter()

	nitroKey := httpsignature.NitroSigner{}

	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{"digest", "date"}

	ps := httpsignature.ParameterizedSignator{
		SignatureParams: sp,
		Signator:        nitroKey,
		Opts:            crypto.Hash(0),
	}

	// middlewares
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)
	r.Use(hlog.NewHandler(*logger))
	r.Use(hlog.UserAgentHandler("user_agent"))
	r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
	r.Use(chiware.Timeout(15 * time.Second))
	logger.Info().Msg("configuration middleware setup")
	// routes
	r.Method("GET", "/", http.HandlerFunc(nitro.EnclaveHealthCheck))
	r.Method("GET", "/health-check", http.HandlerFunc(nitro.EnclaveHealthCheck))
	// setup payments routes
	r.Route("/v1/payments", func(r chi.Router) {
		// Set date header with current date
		r.Use(middleware.SetResponseDate())
		// Sign all payments responses
		r.Use(middleware.SignResponse(ps))
		// Log all payments requests
		r.Use(middleware.RequestLogger(logger))

		// prepare inserts transactions into qldb, returning a document which needs to be submitted by
		// an authorizer
		r.Post(
			"/prepare",
			middleware.InstrumentHandler(
				"PrepareHandler",
				PrepareHandler(s),
			).ServeHTTP,
		)
		logger.Info().Msg("prepare endpoint setup")
		// submit will have an http signature from a known list of public keys
		r.Post(
			"/submit",
			middleware.InstrumentHandler(
				"SubmitHandler",
				s.AuthorizerSignedMiddleware()(SubmitHandler(s)),
			).ServeHTTP)
		logger.Info().Msg("submit endpoint setup")
		// address generation will have an http signature from a known list of public keys
		r.Post(
			"/generatesol",
			middleware.InstrumentHandler(
				"GenerateSolanaAddressHandler",
				s.AuthorizerSignedMiddleware()(GenerateSolanaAddressHandler(s)),
			).ServeHTTP)
		logger.Info().Msg("solana address generation endpoint setup")
		// address approval will have an http signature from a known list of public keys
		r.Post(
			"/approvesol",
			middleware.InstrumentHandler(
				"ApproveSolanaAddressHandler",
				s.AuthorizerSignedMiddleware()(ApproveSolanaAddressHandler(s)),
			).ServeHTTP)
		logger.Info().Msg("solana address approval endpoint setup")

		r.Get(
			"/info",
			middleware.InstrumentHandler(
				"InfoHandler",
				GetConfigurationHandler(s),
			).ServeHTTP,
		)
		logger.Info().Msg("get info endpoint setup")
	})
	return ctx, r
}

// GetConfigurationHandler gets important payments configuration information, attested by nitro.
func GetConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		logger := logging.Logger(ctx, "GetConfigurationHandler")

		resp := &getConfResponse{
			EncryptionKeyARN: service.kmsDecryptKeyArn,
			Environment:      os.Getenv("ENV"),
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
		stateMachine, err := service.StateMachineFromTransaction(ctx, authenticatedState)
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
		// get context from request
		ctx := r.Context()

		var (
			logger        = logging.Logger(ctx, "SubmitHandler")
			submitRequest = &paymentLib.SubmitRequest{}
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
		authenticatedState, err := history.GetAuthenticatedPaymentState(service.verifierStore, submitRequest.DocumentID)
		if err != nil {
			return handlers.WrapError(err, "failed to validate payment state history", http.StatusInternalServerError)
		}

		// attempt authorization on the transaction
		err = service.AuthorizeTransaction(ctx, keyID, authenticatedState)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		err = service.DriveTransaction(ctx, authenticatedState)
		status := authenticatedState.Status

		// Paid and Failed are final states, we don't want the worker to retry.
		// additionally, if we are still in prepared but there is no error,
		// it indicates we did not have sufficient authorizations and should not retry
		code := http.StatusInternalServerError
		if status == paymentLib.Paid || status == paymentLib.Failed || (status == paymentLib.Prepared && err == nil) {
			code = http.StatusOK
		}

		// NOTE: we are intentionally returning an AppError even in the success case as some errors are
		// "permanent" errors indiciating a transaction state machine has reached an end state
		return &handlers.AppError{
			Cause:   err,
			Message: "submitted",
			Code:    code,
			Data: paymentLib.SubmitResponse{
				Status:         status,
				PaymentDetails: authenticatedState.PaymentDetails,
			},
		}
	}
}

// GenerateSolanaAddressHandler.
func GenerateSolanaAddressHandler(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		logger := logging.Logger(ctx, "GenerateSolanaAddressHandler")
		logger.Debug().Msg("handling solana address generation request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapError(err, "error getting identity of transaction authorizer", http.StatusInternalServerError)
		}

		// get the secrets bucket name from environment
		secretsBucketName, ok := service.baseCtx.Value(appctx.EnclaveSecretsBucketNameCTXKey).(string)
		if !ok {
			return handlers.WrapError(err, "no secrets bucket configured", http.StatusInternalServerError)
		}
		chainAddress, err := service.createSolanaAddress(ctx, secretsBucketName, keyID)
		if err != nil {
			return handlers.WrapError(err, "failed to create solana address", http.StatusInternalServerError)
		}

		return &handlers.AppError{
			Cause:   err,
			Message: "key created",
			Code:    http.StatusOK,
			Data:    chainAddress,
		}
	}
}

// ApproveSolanaAddressHandler.
func ApproveSolanaAddressHandler(service *Service) handlers.AppHandler {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger        = logging.Logger(ctx, "ApproveSolanaAddressHandler")
			approvalRequest = &paymentLib.AddressApprovalRequest{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &approvalRequest)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		_, err = govalidator.ValidateStruct(approvalRequest)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logger.Debug().Str("approvals", fmt.Sprintf("%+v", approvalRequest)).Msg("handling approval request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapError(err, "error getting identity of address authorizer", http.StatusInternalServerError)
		}

		chainAddress, err := service.approveSolanaAddress(ctx, approvalRequest.Address, keyID)
		if err != nil {
			return handlers.WrapError(err, "failed to approve solana address", http.StatusInternalServerError)
		}

		return &handlers.AppError{
			Cause:   err,
			Message: "key approved",
			Code:    http.StatusOK,
			Data:    chainAddress,
		}
	}
}
