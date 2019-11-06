package wallet

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for suggestions endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("POST", "/", middleware.HTTPSignedOnly(service)(middleware.InstrumentHandler("PostCreateWallet", PostCreateWallet(service))))
	r.Method("GET", "/{paymentId}", middleware.InstrumentHandler("GetWallet", GetWallet(service)))
	return r
}

// GetPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) GetPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	var publicKey httpsignature.Ed25519PubKey
	// hex encoded public key
	publicKey, err := hex.DecodeString(keyID)
	if err != nil {
		return nil, err
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

// PostCreateWalletResponse includes a ClaimID which can later be used to check the status of the claim
type PostCreateWalletResponse struct {
	Wallet     *Info   `json:"wallet"`
	PrivateKey *string `json:"privateKey"`
}

// GetWalletResponse gets wallet info tied to a wallet id
type GetWalletResponse struct {
	Wallet *Info `json:"wallet"`
}

// PostCreateWalletRequest has possible inputs for the new wallet
type PostCreateWalletRequest struct {
	Provider  string `json:"provider" valid:"in(client)" db:"provider"`
	PublicKey string `json:"publicKey" valid:"required" db:"public_key"`
}

// PostCreateWallet creates a wallet
func PostCreateWallet(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		limit := int64(1024 * 1024 * 10) // 10MiB
		body, err := ioutil.ReadAll(io.LimitReader(r.Body, limit))
		if err != nil {
			return handlers.WrapError(err, "Error reading body", http.StatusBadRequest)
		}

		var req PostCreateWalletRequest
		err = json.Unmarshal(body, &req)
		if err != nil {
			return handlers.WrapError(err, "Error unmarshalling body", http.StatusBadRequest)
		}
		if req.Provider != "client" {
			return handlers.ValidationError("request body", map[string]string{
				"provider": "'" + req.Provider + "' is not supported",
			})
		}
		publicKey, err := middleware.GetKeyID(r.Context())
		if err != nil {
			return handlers.WrapError(err, "Error looking up http signature info", http.StatusBadRequest)
		}
		if req.PublicKey != publicKey {
			return handlers.ValidationError("request signature", map[string]string{
				"publicKey": "publicKey must match signature",
			})
		}

		wallet := CreateWallet(req)
		err = service.datastore.InsertWallet(wallet)
		if err != nil {
			return handlers.WrapError(err, "Error saving wallet", http.StatusServiceUnavailable)
		}

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(&wallet); err != nil {
			panic(err)
		}
		return nil
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

		info, err := service.datastore.GetWallet(paymentID)
		if err != nil {
			return handlers.WrapError(err, "Error getting wallet", http.StatusNotFound)
		}

		// just doing this until another way to track
		if info.AltCurrency == nil {
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		w.WriteHeader(http.StatusOK)
		if err = json.NewEncoder(w).Encode(&info); err != nil {
			panic(err)
		}
		return nil
	})
}

// CreateWallet creates a new set of wallet info
func CreateWallet(req PostCreateWalletRequest) *Info {
	provider := req.Provider // client
	publicKey := req.PublicKey

	var info Info
	info.ID = uuid.NewV4().String()
	info.Provider = provider
	{
		tmp := altcurrency.BAT
		info.AltCurrency = &tmp
	}

	info.PublicKey = publicKey
	return &info
}
