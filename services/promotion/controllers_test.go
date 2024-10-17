//go:build integration

package promotion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/libs/clients/cbr/mock"
	mockreputation "github.com/brave-intl/bat-go/libs/clients/reputation/mock"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/middleware"
	walletutils "github.com/brave-intl/bat-go/libs/wallet"
	"github.com/brave-intl/bat-go/services/wallet"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}

func (suite *ControllersTestSuite) SetupSuite() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	m, err := pg.NewMigrate()
	suite.Require().NoError(err, "Failed to create migrate instance")

	ver, dirty, _ := m.Version()
	if dirty {
		suite.Require().NoError(m.Force(int(ver)))
	}
	if ver > 0 {
		suite.Require().NoError(m.Down(), "Failed to migrate down cleanly")
	}

	suite.Require().NoError(pg.Migrate(), "Failed to fully migrate")

	enableSuggestionJob = true
}

func (suite *ControllersTestSuite) SetupTest() {
	suite.CleanDB()
}

func (suite *ControllersTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *ControllersTestSuite) CleanDB() {
	tables := []string{"claim_drain", "claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *ControllersTestSuite) TestGetPromotions() {

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	cbClient, err := cbr.New()
	suite.Require().NoError(err, "Failed to create challenge bypass client")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	walletID := uuid.NewV4()
	w := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: nil,
		PublicKey:   "-",
		LastBalance: nil,
	}

	err = walletDB.InsertWallet(context.Background(), &w)
	suite.Require().NoError(err, "Failed to insert wallet")

	service := &Service{
		Datastore: pg,
		cbClient:  cbClient,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
	}
	handler := GetAvailablePromotions(service)

	urlWithPlatform := func(platform string) string {
		return fmt.Sprintf("/promotions?paymentId=%s&platform=%s", walletID.String(), platform)
	}

	promotionJSON := func(available bool, promotion *Promotion) string {
		return `{
			"approximateValue": "` + promotion.ApproximateValue.String() + `",
			"available": ` + strconv.FormatBool(available) + `,
			"createdAt": "` + promotion.CreatedAt.Format(time.RFC3339Nano) + `",
			"claimableUntil": "` + promotion.ClaimableUntil.Format(time.RFC3339Nano) + `",
			"expiresAt": "` + promotion.ExpiresAt.Format(time.RFC3339Nano) + `",
			"id": "` + promotion.ID.String() + `",
			"legacyClaimed": ` + strconv.FormatBool(promotion.LegacyClaimed) + `,
			"platform": "` + promotion.Platform + `",
			"publicKeys" : ["1"],
			"suggestionsPerGrant": ` + strconv.Itoa(promotion.SuggestionsPerGrant) + `,
			"type": "ugp",
			"version": 5
		}`
	}

	reqFailure, err := http.NewRequest("GET", urlWithPlatform("noexist"), nil)
	suite.Require().NoError(err, "Failed to create get promotions request")

	reqOSX, err := http.NewRequest("GET", urlWithPlatform("osx"), nil)
	suite.Require().NoError(err, "Failed to create get promotions request")

	reqAndroid, err := http.NewRequest("GET", urlWithPlatform("android"), nil)
	suite.Require().NoError(err, "Failed to create get promotions request")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, reqFailure)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	expectationFailure := `{
		"code":400,
		"message": "Error validating request query parameter",
		"data": {
			"validationErrors": {
				"platform": "platform 'noexist' is not supported"
			}
		}
	}`
	suite.Assert().JSONEq(expectationFailure, rr.Body.String(), "unexpected result")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqOSX)

	suite.Require().Equal(http.StatusOK, rr.Code)
	suite.Assert().JSONEq(`{"promotions": []}`, rr.Body.String(), "unexpected result")

	promotionGeneric, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Failed to create a general promotion")

	// do a get promotion to get the promotion with the claimable until
	promotionGeneric, err = service.Datastore.GetPromotion(promotionGeneric.ID)
	suite.Require().NoError(err, "Failed to get the general promotion")

	promotionDesktop, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(20.0), "desktop")
	suite.Require().NoError(err, "Failed to create osx promotion")

	// do a get promotion to get the promotion with the claimable until
	promotionDesktop, err = service.Datastore.GetPromotion(promotionDesktop.ID)
	suite.Require().NoError(err, "Failed to get the desktop promotion")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqOSX)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedOSX := `{
		"promotions": [
		]
	}`
	suite.Require().JSONEq(expectedOSX, rr.Body.String(), "unexpected result")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqAndroid)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedAndroid := `{
		"promotions": [
		]
	}`
	suite.Assert().JSONEq(expectedAndroid, rr.Body.String(), "unexpected result")

	err = service.Datastore.ActivatePromotion(promotionGeneric)
	suite.Require().NoError(err, "Failed to activate promotion")
	// promotion needs an issuer
	_, err = service.Datastore.InsertIssuer(&Issuer{
		ID:          uuid.NewV4(),
		PromotionID: promotionGeneric.ID,
		Cohort:      "control",
		PublicKey:   `1`,
	})
	suite.Require().NoError(err, "Failed to insert issuer promotion")

	err = service.Datastore.ActivatePromotion(promotionDesktop)
	suite.Require().NoError(err, "Failed to activate promotion")
	// promotion needs an issuer
	_, err = service.Datastore.InsertIssuer(&Issuer{
		ID:          uuid.NewV4(),
		PromotionID: promotionDesktop.ID,
		Cohort:      "control",
		PublicKey:   `1`,
	})
	suite.Require().NoError(err, "Failed to insert issuer promotion")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqOSX)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedOSX = `{
		"promotions": [
			` + promotionJSON(true, promotionGeneric) + `,
			` + promotionJSON(true, promotionDesktop) + `
		]
	}`

	suite.Assert().JSONEq(expectedOSX, rr.Body.String(), "unexpected result")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqAndroid)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedAndroid = `{
		"promotions": [
			` + promotionJSON(true, promotionGeneric) + `
		]
	}`
	suite.Assert().JSONEq(expectedAndroid, rr.Body.String(), "unexpected result")

	statement := `
	insert into claims (promotion_id, wallet_id, approximate_value, legacy_claimed)
	values ($1, $2, $3, true)`
	_, err = pg.RawDB().Exec(statement, promotionDesktop.ID, w.ID, promotionDesktop.ApproximateValue)
	promotionDesktop.LegacyClaimed = true

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqOSX)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedOSX = `{
		"promotions": [
			` + promotionJSON(true, promotionGeneric) + `
		]
	}`
	suite.Assert().JSONEq(expectedOSX, rr.Body.String(), "unexpected result")

	url := fmt.Sprintf("/promotions?paymentId=%s&platform=osx&migrate=true", walletID.String())
	reqOSX, err = http.NewRequest("GET", url, nil)
	suite.Require().NoError(err, "Failed to create get promotions request")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, reqOSX)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expectedOSX = `{
		"promotions": [
			` + promotionJSON(true, promotionGeneric) + `,
			` + promotionJSON(true, promotionDesktop) + `
		]
	}`
	suite.Assert().JSONEq(expectedOSX, rr.Body.String(), "unexpected result")
}

// ClaimPromotion helper that calls promotion endpoint and does assertions
func (suite *ControllersTestSuite) ClaimPromotion(service *Service, w walletutils.Info, privKey httpsignature.Signator,
	promotion *Promotion, blindedCreds []string, claimStatus int) *uuid.UUID {

	handler := middleware.HTTPSignedOnly(service)(ClaimPromotion(service))

	walletID, err := uuid.FromString(w.ID)
	suite.Require().NoError(err)

	claimReq := ClaimRequest{
		WalletID:     walletID,
		BlindedCreds: blindedCreds,
	}

	body, err := json.Marshal(&claimReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/promotion/{promotionId}", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = w.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.SignRequest(privKey, req)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("promotionId", promotion.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if claimStatus != 200 {
		// return early if claim is supposed to fail
		suite.Require().Equal(rr.Code, claimStatus, string(rr.Body.Bytes()))
		return nil
	}
	// if claim was not supposed to fail, or rr.Code is supposed to be ok following line fails
	suite.Require().Equal(http.StatusOK, rr.Code)

	var claimResp ClaimResponse
	err = json.Unmarshal(rr.Body.Bytes(), &claimResp)
	suite.Require().NoError(err)
	return &claimResp.ClaimID
}

func (suite *ControllersTestSuite) TestGetClaimSummary() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")
	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		Datastore: pg,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
	}

	missingWalletID := uuid.NewV4().String()
	body, code := suite.checkGetClaimSummary(service, missingWalletID, "ads")
	suite.Require().Equal(http.StatusNotFound, code, "a 404 is sent back")
	suite.Assert().JSONEq(`{
		"code": 404,
		"message": "Error finding wallet: wallet not found id: '`+missingWalletID+`'"
	}`, body, "an error is returned")

	publicKey := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="
	blindedCreds := jsonutils.JSONStringArray([]string{publicKey})
	walletID := uuid.NewV4().String()
	info := &walletutils.Info{
		ID:         walletID,
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = service.wallet.Datastore.UpsertWallet(context.Background(), info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	// no content returns an empty string on protocol level
	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Assert().Equal(``, body)
	suite.Require().Equal(http.StatusNoContent, code)

	body, code = suite.checkGetClaimSummary(service, "", "ads")
	suite.Assert().JSONEq(`{
		"message": "Error validating query parameter",
		"code": 400,
		"data": {
			"validationErrors": {
				"paymentId": "must be a uuidv4"
			}
		}
	}`, body, "body should return a payment id validation error")
	suite.Require().Equal(http.StatusBadRequest, code)

	// not ignored promotion
	promotion, issuer, claim := suite.setupAdsClaim(service, info, 0)

	_, err = pg.ClaimForWallet(promotion, issuer, info, blindedCreds)
	suite.Require().NoError(err, "apply claim to wallet")

	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Require().Equal(http.StatusOK, code)
	suite.Assert().JSONEq(`{
		"amount": "30",
		"earnings": "30",
		"lastClaim": "`+claim.CreatedAt.Format(time.RFC3339Nano)+`",
		"type": "ads"
	}`, body, "expected a aggregated claim response")

	// ignored promotion (brave transfer
	priorClaim := claim
	promotion, issuer, claim = suite.setupAdsClaim(service, info, 0)
	// set this promotion as a transfer promotion id, and some other random uuid to have more than one
	os.Setenv("BRAVE_TRANSFER_PROMOTION_IDS",
		fmt.Sprintf("%s %s", promotion.ID.String(), "d41ba588-ab18-4300-a180-d2dc01a22371"))

	_, err = pg.ClaimForWallet(promotion, issuer, info, blindedCreds)
	suite.Require().NoError(err, "apply claim to wallet")

	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Require().Equal(http.StatusOK, code)
	// assert you get existing values
	suite.Assert().JSONEq(`{
			"amount": "30",
			"earnings": "30",
			"lastClaim": "`+priorClaim.CreatedAt.Format(time.RFC3339Nano)+`",
			"type": "ads"
		}`, body, "expected a aggregated claim response")

	// not ignored bonus promotion
	promotion, issuer, claim = suite.setupAdsClaim(service, info, 20)

	_, err = pg.ClaimForWallet(promotion, issuer, info, blindedCreds)
	suite.Require().NoError(err, "apply claim to wallet")

	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Require().Equal(http.StatusOK, code)
	suite.Assert().JSONEq(`{
		"amount": "40",
		"earnings": "40",
		"lastClaim": "`+claim.CreatedAt.Format(time.RFC3339Nano)+`",
		"type": "ads"
	}`, body, "expected a aggregated claim response")
}

func (suite *ControllersTestSuite) setupAdsClaim(service *Service, w *walletutils.Info, claimBonus float64) (*Promotion, *Issuer, *Claim) {
	// promo amount can be different than individual grant amount
	promoAmount := decimal.NewFromFloat(25.0)
	promotion, err := service.Datastore.CreatePromotion("ads", 2, promoAmount, "")
	suite.Require().NoError(err, "a promotion could not be created")

	publicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = service.Datastore.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "a promotion should be activated")

	grantAmount := decimal.NewFromFloat(30.0)
	claim, err := service.Datastore.CreateClaim(promotion.ID, w.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	return promotion, issuer, claim
}

func (suite *ControllersTestSuite) checkGetClaimSummary(service *Service, walletID string, claimType string) (string, int) {
	handler := GetClaimSummary(service)
	req, err := http.NewRequest("GET", "/promotion/{claimType}/grants/total?paymentId="+walletID, nil)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("claimType", claimType)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr.Body.String(), rr.Code
}

func (suite *ControllersTestSuite) TestCreatePromotion() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
	}
	var issuerName string
	mockCB.EXPECT().
		CreateIssuer(gomock.Any(), gomock.Any(), gomock.Eq(defaultMaxTokensPerIssuer)).
		DoAndReturn(func(ctx context.Context, name string, maxTokens int) error {
			issuerName = name
			return nil
		})
	mockCB.EXPECT().
		GetIssuer(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, name string) (*cbr.IssuerResponse, error) {
			return &cbr.IssuerResponse{
				Name:      issuerName,
				PublicKey: "",
			}, nil
		})
	handler := CreatePromotion(service)

	createRequest := CreatePromotionRequest{
		Type:      "ugp",
		NumGrants: 10,
		Value:     decimal.NewFromFloat(20.0),
		Platform:  "desktop",
		Active:    true,
	}

	body, err := json.Marshal(&createRequest)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code, fmt.Sprintf("failure body: %s", rr.Body.String()))
}

func (suite *ControllersTestSuite) TestReportClobberedClaims() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "could not connect to db")

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
	}

	handler := PostReportClobberedClaims(service, 1)

	claimIDs := []uuid.UUID{uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	requestPayloadStruct := ClobberedClaimsRequest{
		ClaimIDs: claimIDs,
	}
	payload, err := json.Marshal(&requestPayloadStruct)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/promotions/reportclaimsummary", bytes.NewBuffer(payload))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusOK, rr.Code)

	var clobberedCreds []ClobberedCreds
	err = pg.RawDB().Select(&clobberedCreds, `select * from clobbered_claims;`)
	suite.Require().NoError(err)

	var clobberedCredsIDs []uuid.UUID
	for _, clobberedCred := range clobberedCreds {
		clobberedCredsIDs = append(clobberedCredsIDs, clobberedCred.ID)
	}
	suite.Assert().ElementsMatch(claimIDs, clobberedCredsIDs)
}

func (suite *ControllersTestSuite) TestClobberedClaims_Empty() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		reputationClient: mockReputation,
		cbClient:         mockCB,
	}

	handler := PostReportClobberedClaims(service, 1)
	requestPayloadStruct := ClobberedClaimsRequest{
		ClaimIDs: []uuid.UUID{},
	}
	payload, err := json.Marshal(&requestPayloadStruct)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/v1/promotions/reportclaimsummary", bytes.NewBuffer(payload))
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code)
}

func (suite *ControllersTestSuite) TestPostReportWalletEvent() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "could not connect to db")
	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
	}
	handler := PostReportWalletEvent(service)
	walletID1 := uuid.NewV4()
	walletID2 := uuid.NewV4()

	run := func(walletID uuid.UUID, amount decimal.Decimal, ua string) *httptest.ResponseRecorder {
		requestPayload := BATLossEvent{
			Amount: amount,
		}
		payload, err := json.Marshal(&requestPayload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v1/wallets/"+walletID.String()+"/events/batloss/1", bytes.NewBuffer([]byte(payload)))
		suite.Require().NoError(err)

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("walletId", walletID.String())
		rctx.URLParams.Add("reportId", "1")
		req.Header.Add("User-Agent", ua)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	suite.Require().Equal(http.StatusCreated, run(walletID1, decimal.NewFromFloat(10), "").Code)
	suite.Require().Equal(http.StatusOK, run(walletID1, decimal.NewFromFloat(10), "").Code)
	suite.Require().Equal(http.StatusConflict, run(walletID1, decimal.NewFromFloat(11), "").Code)

	walletEvents := []BATLossEvent{}
	suite.Require().NoError(pg.RawDB().Select(&walletEvents, `select * from bat_loss_events`))
	serializedActual1, err := json.Marshal(&walletEvents)
	serializedExpected1, err := json.Marshal([]BATLossEvent{{
		ID:       walletEvents[0].ID,
		WalletID: walletID1,
		ReportID: 1,
		Amount:   decimal.NewFromFloat(10),
		Platform: "",
	}})
	suite.Require().JSONEq(string(serializedExpected1), string(serializedActual1))

	wallet2Loss := decimal.NewFromFloat(29.4902814)
	macUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36"
	suite.Require().Equal(http.StatusCreated, run(walletID2, decimal.NewFromFloat(29.4902814), macUA).Code)

	walletEvents = []BATLossEvent{}
	suite.Require().NoError(pg.RawDB().Select(&walletEvents, `select * from bat_loss_events;`))
	serializedActual2, err := json.Marshal(&walletEvents)
	serializedExpected2, err := json.Marshal([]BATLossEvent{{
		ID:       walletEvents[0].ID,
		WalletID: walletID1,
		ReportID: 1,
		Amount:   decimal.NewFromFloat(10),
		Platform: "",
	}, {
		ID:       walletEvents[1].ID,
		WalletID: walletID2,
		ReportID: 1,
		Amount:   wallet2Loss,
		Platform: "osx",
	}})
	suite.Require().JSONEq(string(serializedExpected2), string(serializedActual2))
}

// THIS CODE IS A QUICK AND DIRTY HACK
// WE SHOULD DELETE ALL OF THIS AND MOVE OVER TO THE PAYMENT SERVICE ONCE DEMO IS DONE.

// CreateOrder creates orders given the total price, merchant ID, status and items of the order
func (suite *ControllersTestSuite) CreateOrder() (string, error) {
	pg, _, err := NewPostgres()
	tx := pg.RawDB().MustBegin()
	defer pg.RollbackTx(tx)

	var id string

	err = tx.Get(&id, `
			INSERT INTO orders (total_price, merchant_id, status, currency)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, 0.25, "brave.com", "pending", "BAT")

	err = tx.Commit()
	if err != nil {
		return "", err
	}

	return id, nil
}

func (suite *ControllersTestSuite) TestPostReportBAPEvent() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "could not connect to db")
	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
	}
	handler := PostReportBAPEvent(service)
	walletID1 := uuid.NewV4()

	run := func(walletID uuid.UUID, amount decimal.Decimal) *httptest.ResponseRecorder {
		requestPayload := BapReportPayload{
			Amount: amount,
		}
		payload, err := json.Marshal(&requestPayload)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v1/promotions/report-bap", bytes.NewBuffer([]byte(payload)))
		suite.Require().NoError(err)

		rctx := chi.NewRouteContext()
		ctx := middleware.AddKeyID(req.Context(), walletID1.String())
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	suite.Require().Equal(http.StatusOK, run(walletID1, decimal.NewFromFloat(10)).Code)
	suite.Require().Equal(http.StatusConflict, run(walletID1, decimal.NewFromFloat(10)).Code)

	BAPEvents := []BAPReport{}
	suite.Require().NoError(pg.RawDB().Select(&BAPEvents, `select * from bap_report`))
	serializedActual1, err := json.Marshal(&BAPEvents)
	serializedExpected1, err := json.Marshal([]BAPReport{{
		ID:        BAPEvents[0].ID,
		WalletID:  walletID1,
		Amount:    decimal.NewFromFloat(10),
		CreatedAt: BAPEvents[0].CreatedAt,
	}})
	suite.Require().JSONEq(string(serializedExpected1), string(serializedActual1))

}
