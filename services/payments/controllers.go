package payments

import (
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/go-chi/chi"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/requestutils"
)

type configurationHandlerRequest map[appctx.CTXKey]interface{}

type getConfResponse struct {
	PublicKey string `json:"publicKey"`
}

// GetConfigurationHandler - handler to get important payments configuration information,
// namely the ed25519 public key by which we can encrypt the secrets so that only this instance
// of the payments service can decrypt them.
func GetConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		ctx := r.Context()
		var (
			logger = logging.Logger(ctx, "GetConfigurationHandler")
			resp   = &getConfResponse{
				// return the service's public key
				PublicKey: hex.EncodeToString(service.pubKey[:]),
			}
		)

		logger.Debug().Msg("handling configuration request")
		// return ok, at this point all new requests will use the new baseCtx of the service
		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}

// PatchConfigurationHandler - handler to set the location of the current configuration
func PatchConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PatchConfigurationHandler")
			req    = configurationHandlerRequest{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		logger.Info().Str("configurations", fmt.Sprintf("%+v", req)).Msg("handling configuration request")

		// set all the new configurations (will be picked up in future requests by configuration middleware
		service.baseCtx = appctx.MapToContext(service.baseCtx, req)

		if uri, ok := req[appctx.SecretsURICTXKey].(string); ok {
			logger.Info().Str("uri", uri).Msg("secrets location")
			// value of encrypted (nacl box) payments encryption key
			// payments needs to be told what the secret key is for decryption
			// of secrets for it's configuration
			keyCiphertext, ok := service.baseCtx.Value(appctx.PaymentsEncryptionKeyCTXKey).(string)
			if !ok || len(keyCiphertext) == 0 {
				return handlers.WrapError(err, "error decrypting secrets, no key exchange", http.StatusBadRequest)
			}

			senderKey, ok := service.baseCtx.Value(appctx.PaymentsSenderPublicKeyCTXKey).(string)
			if !ok || len(senderKey) == 0 {
				return handlers.WrapError(err, "error decrypting secrets, no sender pubkey", http.StatusBadRequest)
			}

			// go get secrets from secretMgr (handles the kms wrapper key for the object)
			secrets, err := service.secretMgr.RetrieveSecrets(ctx, uri)
			if err != nil {
				logger.Error().Err(err).Msg("error retrieving secrets")
				return handlers.WrapError(err, "error retrieving secrets", http.StatusInternalServerError)
			}

			// decrypt secrets (nacl box to get secret decryption key)
			secretValues, err := service.decryptSecrets(ctx, secrets, keyCiphertext, senderKey)
			if err != nil {
				logger.Error().Err(err).Msg("error decrypting secrets")
				return handlers.WrapError(err, "error decrypting secrets", http.StatusInternalServerError)
			}
			service.baseCtx = appctx.MapToContext(service.baseCtx, secretValues)
		}

		// configure datastore now that we have new ctx
		if err := service.configureDatastore(service.baseCtx); err != nil {
			return handlers.WrapError(err, "error configuring service", http.StatusInternalServerError)
		}

		// return ok, at this point all new requests will use the new baseCtx of the service
		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

// PrepareHandler - handler to get current relative exchange rates
func PrepareHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PrepareHandler")
			req    = []*Transaction{}
			txns   = []*Transaction{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		logger.Debug().Str("request", fmt.Sprintf("%+v", req)).Msg("structure of request")
		// validate the list of transactions

		for _, v := range req {
			_, err = govalidator.ValidateStruct(v)
			if err != nil {
				logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", req)).Msg("failed to validate structure")
				continue // skip txns that are malformed
			}

			txns = append(txns, v)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", txns)).Msg("handling prepare request")

		// returns an enriched list of transactions, which includes the document metadata
		resp, err := service.InsertTransactions(ctx, txns...)
		if err != nil {
			return handlers.WrapError(err, "failed to insert transactions", http.StatusInternalServerError)
		}

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
			req    = []Transaction{}
			txns   = []Transaction{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(ctx, r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}

		for _, v := range req {
			_, err = govalidator.ValidateStruct(v)
			if err != nil {
				logger.Error().Err(err).Str("request", fmt.Sprintf("%+v", req)).Msg("failed to validate structure")
				continue // skip txns that are malformed
			}

			txns = append(txns, v)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", req)).Msg("handling submit request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.AuthorizeTransactions(ctx, keyID, txns...)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		// TODO: check if business logic was met from authorizers table in qldb for this transaction

		for _, t := range txns {
			// perform the custodian submission (channel to worker) if the number of authorizations is appropriate
			service.processTransaction <- t
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

// StatusHandler - handler to perform submission of transactions to custodian
func StatusHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			documentID = chi.URLParam(r, "documentID")
			logger     = logging.Logger(ctx, "StatusHandler")
		)

		logger.Info().Str("documentID", documentID).Msg("handling status request")

		txn, err := service.GetTransactionFromDocID(ctx, documentID)
		if err != nil {
			return handlers.WrapError(err, "failed to get document", http.StatusInternalServerError)
		}
		if txn == nil {
			return handlers.WrapError(err, "no such document", http.StatusNotFound)
		}

		resp := map[string]interface{}{
			"transaction": txn,
		}

		// TODO: get the submission response from qldb add to resp

		// TODO: get the status from the custodian and add to resp
		amount := fromIonDecimal(txn.Amount)
		custodianTransaction, err := custodian.NewTransaction(
			ctx, txn.IdempotencyKey, txn.To, txn.From, altcurrency.BAT, *amount,
		)

		if err != nil {
			logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", txn)).Msg("could not create custodian transaction")
			return handlers.WrapValidationError(err)
		}

		if c, ok := service.custodians[txn.Custodian]; ok {
			// TODO: store the full response from status call of transaction
			_, err = c.GetTransactionsStatus(ctx, custodianTransaction)
			if err != nil {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", txn)).Msg("failed to get transaction status")
				return handlers.WrapError(err, "failed to get status", http.StatusInternalServerError)
			}
		} else {
			logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", txn)).Msg("invalid custodian")
			return handlers.WrapValidationError(fmt.Errorf("invalid custodian"))
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}
