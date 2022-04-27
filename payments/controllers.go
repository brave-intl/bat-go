package payments

import (
	"fmt"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/custodian"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
)

// PrepareHandler - handler to get current relative exchange rates
func PrepareHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
		ctx := r.Context()

		var (
			logger = logging.Logger(ctx, "PrepareHandler")
			req    = []*Transaction{}
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		// validate the list of transactions
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", req)).Msg("handling prepare request")

		// returns an enriched list of transactions, which includes the document metadata
		resp, err := service.InsertTransactions(ctx, req...)
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
		)

		// read the transactions in the body
		err := requestutils.ReadJSON(r.Body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error in request body", http.StatusBadRequest)
		}
		// validate the list of transactions
		_, err = govalidator.ValidateStruct(req)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		logger.Debug().Str("transactions", fmt.Sprintf("%+v", req)).Msg("handling submit request")

		// we have passed the http signature middleware, record who authorized the tx
		keyID, err := middleware.GetKeyID(ctx)
		if err != nil {
			return handlers.WrapValidationError(err)
		}

		err = service.AuthorizeTransactions(ctx, keyID, req...)
		if err != nil {
			return handlers.WrapError(err, "failed to record authorization", http.StatusInternalServerError)
		}

		for _, t := range req {
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

		transaction, err := service.GetTransactionFromDocID(ctx, documentID)
		if err != nil {
			return handlers.WrapError(err, "failed to get document", http.StatusInternalServerError)
		}
		if transaction == nil {
			return handlers.WrapError(err, "no such document", http.StatusNotFound)
		}

		resp := map[string]interface{}{
			"transaction": transaction,
		}

		// TODO: get the submission response from qldb add to resp

		// TODO: get the status from the custodian and add to resp
		custodianTransaction, err := custodian.NewTransaction(
			ctx, transaction.IdempotencyKey, transaction.To, transaction.From, altcurrency.BAT, transaction.Amount,
		)

		if err != nil {
			logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("could not create custodian transaction")
			return handlers.WrapValidationError(err)
		}

		if c, ok := service.custodians[transaction.Custodian]; ok {
			// TODO: store the full response from status call of transaction
			_, err = c.GetTransactionsStatus(ctx, custodianTransaction)
			if err != nil {
				logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("failed to get transaction status")
				return handlers.WrapError(err, "failed to get status", http.StatusInternalServerError)
			}
		} else {
			logger.Error().Err(err).Str("transaction", fmt.Sprintf("%+v", transaction)).Msg("invalid custodian")
			return handlers.WrapValidationError(fmt.Errorf("invalid custodian"))
		}

		return handlers.RenderContent(r.Context(), resp, w, http.StatusOK)
	})
}
