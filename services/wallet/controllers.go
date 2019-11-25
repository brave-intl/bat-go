package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/requestutils"
	"github.com/brave-intl/bat-go/wallet/provider"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

var claimLinkingGeneratorKey = uuid.Must(uuid.FromString("c39b298b-b625-42e9-a463-69c7726e5ddc"))

// Router for suggestions endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/{paymentId}/claim", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostClaimWalletCompat", PostClaimWalletCompat(service))))
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

// ClaimWalletRequest holds the signed transaction to confirm wallet ownership
type ClaimWalletRequest struct {
	SignedTx         string     `json:"signedTx" valid:"base64"`
	AnonymousAddress *uuid.UUID `json:"anonymousAddress"`
}

// PostClaimWalletCompat claims a wallet using ledger pattern
func PostClaimWalletCompat(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		paymentIDString := chi.URLParam(r, "paymentID")
		paymentID, err := uuid.FromString(paymentIDString)
		if err != nil {
			return handlers.ValidationError("url parameter", map[string]string{
				"paymentID": "must be a valid uuidv4",
			})
		}

		var body ClaimWalletRequest
		err = requestutils.ReadJSON(r.Body, &body)
		if err != nil {
			return handlers.ValidationError("request body", map[string]string{
				"body": "unable to ready body",
			})
		}
		anonymousAddress := body.AnonymousAddress

		wallet, err := service.ReadableDatastore().GetWallet(paymentID)
		if err != nil {
			return handlers.WrapError(err, "Error finding wallet", http.StatusInternalServerError)
		}

		userWallet, err := provider.GetWallet(*wallet)
		if err != nil {
			return handlers.WrapError(err, "unable to find wallet", http.StatusNotFound)
		}

		postedTx, err := userWallet.SubmitTransaction(body.SignedTx, false)
		if err != nil {
			return handlers.WrapError(err, "unable to submit transaction", http.StatusBadRequest)
		}

		isMember := postedTx.IsMember
		txType := postedTx.Type
		userID := postedTx.UserID
		if txType != "card" || !isMember || userID == nil {
			return handlers.WrapError(err, "unable submit transaction", http.StatusBadRequest)
		}

		providerLinkingID := uuid.NewV5(claimLinkingGeneratorKey, userID.String())
		if wallet.ProviderLinkingID != nil {
			// check if the member matches the associated member
			if !uuid.Equal(*wallet.ProviderLinkingID, providerLinkingID) {
				return handlers.WrapError(errors.New("wallets do not match"), "unable to match wallets", http.StatusConflict)
			}
			if anonymousAddress != nil && !uuid.Equal(*anonymousAddress, *wallet.AnonymousAddress) {
				err := service.datastore.SetAnonymousAddress(paymentID, *anonymousAddress)
				if err != nil {
					return handlers.WrapError(err, "unable to set anonymous address", http.StatusInternalServerError)
				}
			}
		} else {
			err := service.datastore.LinkWallet(paymentID, providerLinkingID)
			if err != nil {
				return handlers.WrapError(err, "unable to link wallets", http.StatusInternalServerError)
			}
		}

		_, err = userWallet.SubmitTransaction(body.SignedTx, true)
		if err != nil {
			return handlers.WrapError(err, "unable to submit transaction", http.StatusServiceUnavailable)
		}
		return nil
	})
}
