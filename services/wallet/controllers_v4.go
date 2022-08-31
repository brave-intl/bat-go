package wallet

import (
	"crypto"
	"encoding/json"
	"errors"
	"github.com/asaskevich/govalidator"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"net/http"
)

type CreateBraveWalletV4Request struct {
	Geolocation string `json:"geolocation"`
}

// CreateBraveWalletV4 creates a brave wallet. This endpoint takes a geolocation as part of the request.
func CreateBraveWalletV4(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		verifier := httpsignature.ParameterizedKeystoreVerifier{
			SignatureParams: httpsignature.SignatureParams{
				Algorithm: httpsignature.ED25519,
				Headers:   []string{"digest", "(request-target)"},
			},
			Keystore: &DecodeEd25519Keystore{},
			Opts:     crypto.Hash(0),
		}

		// perform validation based on public key that the user submits
		ctx, publicKey, err := verifier.VerifyRequest(r)
		if err != nil {
			return handlers.WrapError(err, "invalid http signature", http.StatusForbidden)
		}

		var c CreateBraveWalletV4Request
		err = json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			return handlers.WrapError(err, "error decoding request body", http.StatusBadRequest)
		}

		if !govalidator.IsISO3166Alpha2(c.Geolocation) {
			return handlers.WrapError(err, "error gelocation must be ISO3166Alpha2 format", http.StatusBadRequest)
		}

		info, err := s.CreateBraveWallet(ctx, publicKey, c.Geolocation)
		if err != nil {
			logging.FromContext(ctx).Error().Err(err).
				Msg("error creating brave wallet")
			switch {
			case errors.Is(err, errGeoLocationDisabled):
				return handlers.WrapError(errGeoLocationDisabled,
					"error creating brave wallet", http.StatusForbidden)
			default:
				return handlers.WrapError(errorutils.ErrInternalServerError,
					"error creating brave wallet", http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusCreated)
	}
}
