package subscriptions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/brave-intl/bat-go/payment"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/logging"
	servertiming "github.com/brave-intl/go-server-timing"
)

var (
	errUnexpectedSkuCookie     = errors.New("unexpected sku cookie")
	errUnexpectedQueryUnescpae = errors.New("unexpected query unscape")
	errUnexpectedBase64Decode  = errors.New("unexpected base64 decode")
	errMalformedSkuToken       = errors.New("malformed sku token")
	errUnexpectedVerifyCredReq = errors.New("unexpected decoded credential")
	errInvalidCredentials      = errors.New("invalid credentials reject by skus ervice")
)

func getSkuCookieName(prefix, sku string) string {
	return prefix + "sku#" + sku
}

func VerifyAnonOptional(validSKUs []string, skuClient SkuClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := logging.Logger(ctx, "VerifyAnonOptional")

			var sku string
			var c *http.Cookie
			var err error

			for _, sku = range validSKUs {
				for _, prefix := range []string{"__Secure-", "__Host-"} {
					cookieName := getSkuCookieName(prefix, sku)
					c, err = r.Cookie(cookieName)
					if err != nil {
						cookieReg := regexp.MustCompile(".*cookie not present.*")
						if !cookieReg.MatchString(err.Error()) {
							logger.Error().Err(err).Msg("failed to verify request")
							ae := handlers.AppError{
								Cause:   errUnexpectedSkuCookie,
								Message: "Contact tech support",
								Code:    http.StatusInternalServerError,
							}
							ae.ServeHTTP(w, r)
							return
						}
					}
					if c != nil {
						// Unset the cookie
						cookie := http.Cookie{
							Name:     cookieName,
							Value:    "",
							Expires:  time.Now().Add(time.Hour * time.Duration(-1)),
							SameSite: http.SameSiteStrictMode,
							Secure:   true,
							Path:     c.Path,
						}
						http.SetCookie(w, &cookie)

						break
					}
				}
			}

			if c != nil {
				decodedValue, err := url.QueryUnescape(c.Value)
				if err != nil {
					logger.Error().Err(err).Msg(errUnexpectedQueryUnescpae.Error())
					ae := handlers.AppError{
						Cause:   errUnexpectedQueryUnescpae,
						Message: err.Error(),
						Code:    http.StatusInternalServerError,
					}
					ae.ServeHTTP(w, r)
					return
				}
				decodedBytes, err := base64.StdEncoding.DecodeString(decodedValue)
				if err != nil {
					cookieReg := regexp.MustCompile(".*base64.*")
					if !cookieReg.MatchString(err.Error()) {
						logger.Error().Err(err).Msg(errUnexpectedBase64Decode.Error())
						ae := handlers.AppError{
							Cause:   errUnexpectedBase64Decode,
							Message: err.Error(),
							Code:    http.StatusInternalServerError,
						}
						ae.ServeHTTP(w, r)
						return
					}
					logger.Warn().Msg(errMalformedSkuToken.Error())
					ae := handlers.AppError{
						Cause:   errMalformedSkuToken,
						Message: err.Error(),
						Code:    http.StatusBadRequest,
					}
					ae.ServeHTTP(w, r)
					return
				}
				credReq := payment.VerifyCredentialRequestV1{}
				err = json.Unmarshal(decodedBytes, &credReq)
				if err != nil {
					logger.Error().Err(err).Msg(errUnexpectedVerifyCredReq.Error())
					ae := handlers.AppError{
						Cause:   errUnexpectedVerifyCredReq,
						Message: err.Error(),
						Code:    http.StatusInternalServerError,
					}
					ae.ServeHTTP(w, r)
					return
				}

				timing := servertiming.FromContext(r.Context())
				verifyMetric := timing.NewMetric("skus-backend").Start()
				// NOTE VerifyCred ensures sku and merchantID must match credential
				// and that by getting here we have already ensured that sku is an allowed value
				err = skuClient.VerifyCred(credReq, "brave.com", sku)
				verifyMetric.Stop()
				if err != nil {
					logger.Error().Err(err).Msg(errInvalidCredentials.Error())
					ae := handlers.AppError{
						Cause:   errInvalidCredentials,
						Message: err.Error(),
						Code:    http.StatusForbidden,
					}
					ae.ServeHTTP(w, r)
					return
				}

				ctx = context.WithValue(ctx, AuthContextKey, sku)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
