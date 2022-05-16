package payments

import (
	"encoding/hex"
	"fmt"
	"net/http"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/requestutils"
)

type configurationHandlerRequest map[appctx.CTXKey]interface{}

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

		// return ok, at this point all new requests will use the new baseCtx of the service
		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	})
}

type getConfResponse struct {
	PublicKey string `json:"publicKey"`
}

// GetConfigurationHandler - handler to get important payments configuration information,
// namely the ed25519 public key by which we can encrypt the secrets so that only this instance
// of the payments service can decrypt them.
func GetConfigurationHandler(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		// get context from request
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
