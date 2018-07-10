package controllers

import (
	"encoding/json"
	"fmt"
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
	"github.com/pressly/lg"
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
		r.Method("POST", "/", chiware.Throttle(int(throttle))(middleware.InstrumentHandlerFunc("RedeemGrants", RedeemGrants)))
	} else {
		r.Post("/", middleware.InstrumentHandlerFunc("RedeemGrants", RedeemGrants))
	}
	r.Put("/{grantId}", middleware.InstrumentHandlerFunc("ClaimGrant", ClaimGrant))
	return r
}

// ClaimGrant is the handler for claiming grants
func ClaimGrant(w http.ResponseWriter, r *http.Request) {
	log := lg.Log(r.Context())
	defer utils.PanicCloser(r.Body)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	var req grant.ClaimGrantRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		errMsg := fmt.Sprintf("Error unmarshalling body: %v", err)
		log.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	_, err = govalidator.ValidateStruct(req)
	if err != nil {
		errMsg := fmt.Sprintf("Error validating request payload: %v", err)
		log.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	if grantID := chi.URLParam(r, "grantId"); grantID != "" {
		if !govalidator.IsUUIDv4(grantID) {
			errMsg := fmt.Sprintf("Error validating request url parameter: grantId must be a uuidv4")
			log.Error(errMsg)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		err = req.Claim(r.Context(), grantID)
		if err != nil {
			errMsg := fmt.Sprintf("Error claiming grant: %v", err)
			log.Error(errMsg)
			// FIXME not all errors are 4xx
			http.Error(w, errMsg, http.StatusBadRequest)
			return
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

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
}

// RedeemGrants is the handler for redeeming one or more grants
func RedeemGrants(w http.ResponseWriter, r *http.Request) {
	log := lg.Log(r.Context())
	defer utils.PanicCloser(r.Body)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}

	var req grant.RedeemGrantsRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		errMsg := fmt.Sprintf("Error unmarshalling body: %v", err)
		log.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	_, err = govalidator.ValidateStruct(req)
	if err != nil {
		errMsg := fmt.Sprintf("Error validating grant: %v", err)
		log.Error(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	redeemedIDs, err := grant.GetRedeemedIDs(r.Context(), req.Grants)
	if err != nil || len(redeemedIDs) > 0 {
		log.Error("Grants have already been redeemed")
		respond(w, RedeemError{Err: "alreadyRedeemedError", Data: redeemedIDs})
		return
	}

	txInfo, err := req.Redeem(r.Context())
	if err != nil {
		errMsg := fmt.Sprintf("Error redeeming grant: %v", err)
		log.Error(errMsg)
		// FIXME not all errors are 4xx
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	respond(w, txInfo)
}

// RedeemError holds error info as well as a data structure
type RedeemError struct {
	Err  string   `json:"error"`
	Data []string `json:"data"`
}

func (e *RedeemError) Error() string {
	return fmt.Sprintf("%s: %v", e.Err, e.Data)
}

func respond(w http.ResponseWriter, data interface{}) {
	w.Header().Set("content-type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		panic(err)
	}
}
