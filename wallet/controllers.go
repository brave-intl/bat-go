package wallet

import (
	"context"
	"crypto"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/go-chi/chi"
	uuid "github.com/satori/go.uuid"
)

// Router for suggestions endpoints
func Router(service *Service) chi.Router {
	r := chi.NewRouter()
	r.Method("GET", "/{paymentId}", middleware.InstrumentHandler("GetWallet", GetWallet(service)))
	return r
}

// LookupPublicKey based on the HTTP signing keyID, which in our case is the walletID
func (service *Service) LookupPublicKey(ctx context.Context, keyID string) (*httpsignature.Verifier, error) {
	walletID, err := uuid.FromString(keyID)
	if err != nil {
		return nil, errorutils.Wrap(err, "KeyID format is invalid")
	}

	wallet, err := service.GetWallet(walletID)
	if err != nil {
		return nil, errorutils.Wrap(err, "error getting wallet")
	}

	if wallet == nil {
		return nil, nil
	}

	var publicKey httpsignature.Ed25519PubKey
	if len(wallet.PublicKey) > 0 {
		var err error
		publicKey, err = hex.DecodeString(wallet.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	tmp := httpsignature.Verifier(publicKey)
	return &tmp, nil
}

// LinkWalletRequest holds the data necessary to update a wallet with an anonymous address
type LinkWalletRequest struct {
	SignedTx         string     `json:"signedTx"`
	AnonymousAddress *uuid.UUID `json:"anonymousAddress"`
}

// GetWalletResponse gets wallet info tied to a wallet id
type GetWalletResponse struct {
	Wallet *walletutils.Info `json:"wallet"`
}

func validateHTTPSignature(ctx context.Context, r *http.Request, signature string) (string, error) {
	// validate that the signature in the header is valid based on public key provided
	var s httpsignature.Signature
	err := s.UnmarshalText([]byte(signature))
	if err != nil {
		return "", fmt.Errorf("invalid signature: %w", err)
	}

	// Override algorithm and headers to those we want to enforce
	s.Algorithm = httpsignature.ED25519
	s.Headers = []string{"digest", "(request-target)"}
	var publicKey httpsignature.Ed25519PubKey
	if len(s.KeyID) > 0 {
		var err error
		publicKey, err = hex.DecodeString(s.KeyID)
		if err != nil {
			return "", fmt.Errorf("failed to hex decode public key: %w", err)
		}
	}
	pubKey := httpsignature.Verifier(publicKey)
	if err != nil {
		return "", err
	}
	if pubKey == nil {
		return "", errors.New("invalid public key")
	}

	valid, err := s.Verify(pubKey, crypto.Hash(0), r)

	if err != nil {
		return "", fmt.Errorf("failed to verify signature: %w", err)
	}
	if !valid {
		return "", errors.New("invalid signature")
	}
	return s.KeyID, nil
}

// GetWallet retrieves wallet information
func GetWallet(service *Service) handlers.AppHandler {
	return handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		var ctx = r.Context()
		logger, err := appctx.GetLogger(ctx)
		if err != nil {
			// no logger, setup
			ctx, logger = logging.SetupLogger(ctx)
			r = r.WithContext(ctx)
		}
		paymentIDParam := chi.URLParam(r, "paymentId")
		paymentID, err := uuid.FromString(paymentIDParam)

		if err != nil {
			return handlers.ValidationError("request url parameter", map[string]string{
				"paymentId": "paymentId '" + paymentIDParam + "' is not supported",
			})
		}

		info, err := service.Datastore.GetWallet(paymentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				logger.Info().Err(err).Str("paymentID", paymentID.String()).Msg("wallet not found")
				return handlers.WrapError(err, "no such wallet", http.StatusNotFound)
			}
			return handlers.WrapError(err, "Error getting wallet", http.StatusInternalServerError)
		}

		// just doing this until another way to track
		if info.AltCurrency == nil {
			tmp := altcurrency.BAT
			info.AltCurrency = &tmp
		}

		return handlers.RenderContent(r.Context(), info, w, http.StatusOK)
	})
}
