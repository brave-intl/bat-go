package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/grant"
	"github.com/brave-intl/bat-go/middleware"
	"github.com/go-chi/chi"
	"github.com/pressly/lg"
)

func GrantsRouter() chi.Router {
	r := chi.NewRouter()
	if os.Getenv("ENV") == "production" {
		r.Use(middleware.SimpleTokenAuthorizedOnly)
	}
	r.Post("/", middleware.InstrumentHandler("RedeemGrants", RedeemGrants))
	return r
}

func RedeemGrants(w http.ResponseWriter, r *http.Request) {
	log := lg.Log(r.Context())

	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error("Error reading body: %v", err)
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

	txInfo, err := req.Redeem(r.Context())
	if err != nil {
		errMsg := fmt.Sprintf("Error redeeming grant: %v", err)
		log.Error(errMsg)
		// FIXME not all errors are 4xx
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(txInfo); err != nil {
		panic(err)
	}
}
