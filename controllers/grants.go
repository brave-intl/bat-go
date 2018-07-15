package controllers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/datastore"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils"
	"github.com/garyburd/redigo/redis"
	raven "github.com/getsentry/raven-go"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
)

// GrantsRouter is the router for grant endpoints
func GrantsRouter() chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") == "production" {
		r.Use(middleware.SimpleTokenAuthorizedOnly)
	}
	if len(os.Getenv("THROTTLE_GRANT_REQUESTS")) > 0 {
		throttle, err := strconv.ParseInt(os.Getenv("THROTTLE_GRANT_REQUESTS"), 10, 0)
		if err != nil {
			panic("THROTTLE_GRANT_REQUESTS was provided but not a valid number")
		}
		r.Method("POST", "/", chiware.Throttle(int(throttle))(middleware.InstrumentHandler("RedeemGrants", utils.AppHandler(RedeemGrants))))
	} else {
		r.Method("POST", "/", middleware.InstrumentHandler("RedeemGrants", utils.AppHandler(RedeemGrants)))
	}
	r.Method("PUT", "/{grantId}", middleware.InstrumentHandler("ClaimGrant", utils.AppHandler(ClaimGrant)))
	return r
}

// ClaimGrant is the handler for claiming grants
func ClaimGrant(w http.ResponseWriter, r *http.Request) *utils.AppError {
	defer utils.PanicCloser(r.Body)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return utils.WrapError("Error reading body", err)
	}

	var req grant.ClaimGrantRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		return utils.WrapError("Error unmarshalling body", err)
	}
	_, err = govalidator.ValidateStruct(req)
	if err != nil {
		return utils.WrapValidationError(err)
	}

	if grantID := chi.URLParam(r, "grantId"); grantID != "" {
		if !govalidator.IsUUIDv4(grantID) {
			return &utils.AppError{
				Message: "Error validating request url parameter",
				Code:    http.StatusBadRequest,
				Data: map[string]interface{}{
					"validationErrors": map[string]string{
						"grantId": "grantId must be a uuidv4",
					},
				},
			}
		}

		err = req.Claim(r.Context(), grantID)
		if err != nil {
			// FIXME not all errors are 4xx
			utils.WrapError("Error claiming grant", err)
		}
	}

	conn := datastore.GetRedisConn(r.Context())
	// FIXME TODO clean this up via a better abstraction
	if _, err = redis.Int((*conn).Do("ZINCRBY", "count:claimed:ip", "1", r.RemoteAddr)); err != nil {
		raven.CaptureMessage("Could not increment claim count for ip.", map[string]string{"IP": r.RemoteAddr})
	}
	if err = (*conn).Close(); err != nil {
		raven.CaptureMessage("Could not cleanly close db conn post ip increment.", map[string]string{})
	}

	w.WriteHeader(http.StatusOK)
	return nil
}

// RedeemGrants is the handler for redeeming one or more grants
func RedeemGrants(w http.ResponseWriter, r *http.Request) *utils.AppError {
	defer utils.PanicCloser(r.Body)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return utils.WrapError("Error reading body", err)
	}

	var req grant.RedeemGrantsRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		return utils.WrapError("Error unmarshalling body", err)
	}
	_, err = govalidator.ValidateStruct(req)
	if err != nil {
		return utils.WrapValidationError(err)
	}

	redeemedIDs, err := grant.GetRedeemedIDs(r.Context(), req.Grants)
	if err != nil {
		return utils.WrapError("Error checking grant redemption status", err)
	}

	if len(redeemedIDs) > 0 {
		return &utils.AppError{
			Message: "One or more grants have already been redeemed",
			Code:    http.StatusGone,
			Data:    map[string]interface{}{"redeemedIDs": redeemedIDs},
		}
	}

	txInfo, err := req.Redeem(r.Context())
	if err != nil {
		// FIXME not all errors are 4xx
		return utils.WrapError("Error redeeming grant", err)
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(txInfo); err != nil {
		panic(err)
	}
	return nil
}
