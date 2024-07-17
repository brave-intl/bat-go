package wallet

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/go-chi/chi"
)

var (
	errGeoCountryFormat  = errors.New("error geo country format must be ISO3166Alpha2")
	errGeoAlreadySet     = errors.New("error geo country has already been set for rewards wallet")
	errPaymentIDMismatch = errors.New("error payment id does not match http signature key id")
)

// V4Request contains the fields for making v4 wallet requests.
type V4Request struct {
	GeoCountry string `json:"geoCountry"`
}

// V4Response contains the fields for v4 wallet responses.
type V4Response struct {
	PaymentID string `json:"paymentId"`
}

// CreateWalletV4 creates a brave rewards wallet. This endpoint takes a geo country as part of the request
// that must be ISO3166Alpha2 format.
func CreateWalletV4(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		logger := logging.Logger(r.Context(), "wallet.CreateWalletV4")

		verifier := httpsignature.ParameterizedKeystoreVerifier{
			SignatureParams: httpsignature.SignatureParams{
				Algorithm: httpsignature.ED25519,
				Headers: []string{
					httpsignature.DigestHeader,
					httpsignature.RequestTargetHeader,
				},
			},
			Keystore: &DecodeEd25519Keystore{},
		}

		ctx, publicKey, err := verifier.VerifyRequest(r)
		if err != nil {
			logger.Error().Err(err).Msg("error creating rewards wallet")
			return handlers.WrapError(err, "error creating rewards wallet", http.StatusUnauthorized)
		}

		var request V4Request
		err = json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			logger.Error().Err(err).Msg("error creating rewards wallet")
			return handlers.WrapError(err, "error creating rewards wallet", http.StatusBadRequest)
		}

		if !govalidator.IsISO3166Alpha2(request.GeoCountry) {
			logger.Error().Err(errGeoCountryFormat).Msg("error creating rewards wallet")
			return handlers.WrapError(errGeoCountryFormat, "error creating rewards wallet", http.StatusBadRequest)
		}

		info, err := s.CreateRewardsWallet(ctx, publicKey, request.GeoCountry)
		if err != nil {
			logger.Error().Err(err).
				Msg("error creating rewards wallet")

			var errorBundle *errorutils.ErrorBundle
			if errors.As(err, &errorBundle) {
				logger.Error().
					Str("error_bundle", errorBundle.DataToString()).
					Msg("error creating rewards wallet")
			}

			switch {
			case errors.Is(err, errRewardsWalletAlreadyExists):
				return handlers.WrapError(errRewardsWalletAlreadyExists,
					"error creating rewards wallet", http.StatusConflict)
			case errors.Is(err, errGeoCountryDisabled):
				return handlers.WrapError(errGeoCountryDisabled,
					"error creating rewards wallet", http.StatusForbidden)
			default:
				return handlers.WrapError(errorutils.ErrInternalServerError,
					"error creating rewards wallet", http.StatusInternalServerError)
			}
		}

		response := V4Response{
			PaymentID: info.ID,
		}

		return handlers.RenderContent(ctx, response, w, http.StatusCreated)
	}
}

// UpdateWalletV4 updates a brave rewards wallet. This endpoint takes a geo country as part of the request that must
// be ISO3166Alpha2 format.
func UpdateWalletV4(s *Service) func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		logger := logging.Logger(r.Context(), "wallet.UpdateWalletV4")

		paymentID := chi.URLParam(r, "paymentID")
		if paymentID == "" {
			logger.Error().Err(errorutils.ErrBadRequest).Msg("error updating rewards wallet")
			return handlers.ValidationError("error validating paymentID url parameter",
				map[string]interface{}{"paymentID": errorutils.ErrBadRequest.Error()})
		}

		keyID, err := middleware.GetKeyID(r.Context())
		if err != nil {
			logger.Error().Err(err).Msg("error updating rewards wallet")
			return handlers.ValidationError("error retrieving keyID from signature",
				map[string]interface{}{"keyID": err.Error()})
		}

		if paymentID != keyID {
			logger.Error().Err(errPaymentIDMismatch).Msg("error updating rewards wallet")
			return handlers.WrapError(errPaymentIDMismatch, "error updating rewards wallet", http.StatusForbidden)
		}

		var request V4Request
		err = json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			logger.Error().Err(err).Msg("error updating rewards wallet")
			return handlers.WrapError(err, "error updating rewards wallet", http.StatusBadRequest)
		}

		if !govalidator.IsISO3166Alpha2(request.GeoCountry) {
			logger.Error().Err(errGeoCountryFormat).Msg("error updating rewards wallet")
			return handlers.WrapError(errGeoCountryFormat, "error updating rewards wallet", http.StatusBadRequest)
		}

		// Currently we do not check for the wallet existence as the middleware LookupVerifier covers this.
		upsertReputationSummary := func() (interface{}, error) {
			return nil, s.repClient.UpsertReputationSummary(r.Context(), paymentID, request.GeoCountry)
		}

		_, err = s.retry(r.Context(), upsertReputationSummary, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			logger.Error().Err(err).Msg("error updating rewards wallet")
			var errorBundle *errorutils.ErrorBundle
			if errors.As(err, &errorBundle) {
				logger.Error().
					Str("error bundle", errorBundle.DataToString()).
					Msg("error updating rewards wallet")
				if httpState, ok := errorBundle.Data().(clients.HTTPState); ok {
					if httpState.Status == http.StatusBadRequest {
						return handlers.WrapError(errorutils.ErrBadRequest,
							"error updating rewards wallet", http.StatusBadRequest)
					}
					if httpState.Status == http.StatusConflict {
						return handlers.WrapError(errGeoAlreadySet,
							"error updating rewards wallet", http.StatusConflict)
					}
				}
			}
			return handlers.WrapError(errorutils.ErrInternalServerError, "error updating rewards wallet",
				http.StatusInternalServerError)
		}

		return handlers.RenderContent(r.Context(), nil, w, http.StatusOK)
	}
}

// GetWalletV4 is the same as get wallet v3, but we are now requiring http signatures for get wallet requests
func GetWalletV4(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return GetWalletV3(w, r)
}

// GetUpholdWalletBalanceV4 produces an http handler for the service s which handles balance inquiries of uphold wallets
func GetUpholdWalletBalanceV4(w http.ResponseWriter, r *http.Request) *handlers.AppError {
	return GetUpholdWalletBalanceV3(w, r)
}
