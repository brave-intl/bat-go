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

var errGeoCountryFormat = errors.New("error geo country format must be ISO3166Alpha2")

type CreateWalletV4Request struct {
	GeoCountry string `json:"geo_country"`
}

// CreateWalletV4 creates a brave rewards wallet. This endpoint takes a geo country as part of the request that must
// be ISO3166Alpha2 format.
func CreateWalletV4(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		verifier := httpsignature.ParameterizedKeystoreVerifier{
			SignatureParams: httpsignature.SignatureParams{
				Algorithm: httpsignature.ED25519,
				Headers:   []string{"digest", "(request-target)"},
			},
			Keystore: &DecodeEd25519Keystore{},
			Opts:     crypto.Hash(0),
		}

		logger := logging.Logger(r.Context(), "wallet.CreateWalletV4")

		// perform validation based on public key that the user submits
		ctx, publicKey, err := verifier.VerifyRequest(r)
		if err != nil {
			logger.Error().Err(err).Msg("error creating rewards wallet")
			return handlers.WrapError(err, "error creating rewards wallet", http.StatusUnauthorized)
		}

		var c CreateWalletV4Request
		err = json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			logger.Error().Err(err).Msg("error creating rewards wallet")
			return handlers.WrapError(err, "error creating rewards wallet", http.StatusBadRequest)
		}

		if !govalidator.IsISO3166Alpha2(c.GeoCountry) {
			logger.Error().Err(errGeoCountryFormat).Msg("error creating rewards wallet")
			return handlers.WrapError(errGeoCountryFormat, "error creating rewards wallet", http.StatusBadRequest)
		}

		info, err := s.CreateRewardsWallet(ctx, publicKey, c.GeoCountry)
		if err != nil {
			logger.Error().Err(err).Msg("error creating rewards wallet")
			switch {
			case errors.Is(err, errWalletAlreadyExists):
				return handlers.WrapError(errWalletAlreadyExists,
					"error creating rewards wallet", http.StatusConflict)
			case errors.Is(err, errGeoCountryDisabled):
				return handlers.WrapError(errGeoCountryDisabled,
					"error creating rewards wallet", http.StatusForbidden)
			default:
				return handlers.WrapError(errorutils.ErrInternalServerError,
					"error creating rewards wallet", http.StatusInternalServerError)
			}
		}

		return handlers.RenderContent(ctx, infoToResponseV3(info), w, http.StatusCreated)
	}
}
