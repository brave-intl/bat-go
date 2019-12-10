package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"os"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for suggestions endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	PostLinkWalletCompatHandler := middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostLinkWalletCompat", PostLinkWalletCompat(service)))
	if os.Getenv("ENV") != "local" {
		PostLinkWalletCompatHandler = middleware.SimpleTokenAuthorizedOnly(PostLinkWalletCompatHandler)
	}
	r.Method("POST", "/{paymentId}/link", PostLinkWalletCompatHandler)
	return r
}

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	var publicKey httpsignature.Ed25519PubKey
	// hex encoded public key
	publicKey, err := hex.DecodeString(keyID)
	if err != nil {
		return nil, err
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

// LinkWalletRequest holds the data necessary to update a wallet with an anonymous address
type LinkWalletRequest struct {
	ProviderLinkingID *uuid.UUID `json:"providerLinkingId"`
	AnonymousAddress  *uuid.UUID `json:"anonymousAddress"`
}

// PostLinkWalletCompat links wallets using provided ids
func PostLinkWalletCompat(service *Service) handlers.AppHandler {
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
		anonymousAddress := body.AnonymousAddress

		wallet, err := service.ReadableDatastore().GetWallet(paymentID)
		if err != nil {
			return handlers.WrapError(err, "Error finding wallet", http.StatusNotFound)
		}

		if wallet.ProviderLinkingID != nil {
			// check if the member matches the associated member
			if !uuid.Equal(*wallet.ProviderLinkingID, *body.ProviderLinkingID) {
				return handlers.WrapError(errors.New("wallets do not match"), "unable to match wallets", http.StatusConflict)
			}
			if anonymousAddress != nil && !uuid.Equal(*anonymousAddress, *wallet.AnonymousAddress) {
				err := service.datastore.SetAnonymousAddress(paymentID, *anonymousAddress)
				if err != nil {
					return handlers.WrapError(err, "unable to set anonymous address", http.StatusInternalServerError)
				}
			}
		} else {
			err := service.datastore.LinkWallet(paymentID, *body.ProviderLinkingID, *anonymousAddress)
			if err != nil {
				return handlers.WrapError(err, "unable to link wallets", http.StatusInternalServerError)
			}
		}
		return nil
	})
}
