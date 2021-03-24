// +build integration

package promotion

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/clients"
	mockbitflyer "github.com/brave-intl/bat-go/utils/clients/bitflyer/mock"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	cbr "github.com/brave-intl/bat-go/utils/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	mockreputation "github.com/brave-intl/bat-go/utils/clients/reputation/mock"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	walletutils "github.com/brave-intl/bat-go/utils/wallet"
	"github.com/brave-intl/bat-go/utils/wallet/provider/uphold"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
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

	promotionDesktop, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(20.0), "desktop")
	suite.Require().NoError(err, "Failed to create osx promotion")

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

func (suite *ControllersTestSuite) ClaimGrant(
	service *Service,
	w walletutils.Info,
	privKey crypto.Signer,
	promotion *Promotion,
	blindedCreds []string,
	claimStatus int,
	// promoActive bool,
) *uuid.UUID {
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

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = w.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
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

func (suite *ControllersTestSuite) WaitForClaimToPropagate(service *Service, promotion *Promotion, claimID *uuid.UUID) {
	handler := GetClaim(service)

	req, err := http.NewRequest("GET", "/promotion/{promotionId}/claims/{claimId}", nil)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("promotionId", promotion.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	ctx, _ := context.WithTimeout(req.Context(), 500*time.Millisecond)
	cID := *claimID
	rctx.URLParams.Add("claimId", cID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	for rr.Code != http.StatusOK {
		if rr.Code == http.StatusBadRequest {
			break
		}
		select {
		case <-ctx.Done():
			break
		default:
			time.Sleep(50 * time.Millisecond)
			rr = httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	}
	suite.Require().Equal(http.StatusOK, rr.Code, "Async signing timed out")

	var getClaimResp GetClaimResponse
	err = json.Unmarshal(rr.Body.Bytes(), &getClaimResp)
	suite.Require().NoError(err)
	suite.Require().Equal(promotion.SuggestionsPerGrant, len(getClaimResp.SignedCreds), "Signed credentials should have the same length")
}

func (suite *ControllersTestSuite) TestClaimGrant() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	cbClient, err := cbr.New()
	suite.Require().NoError(err, "Failed to create challenge bypass client")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)

	service := &Service{
		Datastore: pg,
		cbClient:  cbClient,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
	}

	promotion, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	blindedCreds := make([]string, promotion.SuggestionsPerGrant)
	for i := range blindedCreds {
		blindedCreds[i] = "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	}

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	handler := GetAvailablePromotions(service)
	req, err := http.NewRequest("GET", fmt.Sprintf("/promotions?paymentId=%s&platform=osx", walletID.String()), nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
	expected := `{
		"promotions": []
	}`
	suite.Assert().JSONEq(expected, rr.Body.String(), "Expected public key to appear in promotions endpoint")

	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)

	promotion, _, claim := suite.setupAdsClaim(service, &info, 0)

	handler2 := middleware.HTTPSignedOnly(service)(ClaimPromotion(service))

	// blindedCreds should be the wrong length
	claimReq := ClaimRequest{
		WalletID:     walletID,
		BlindedCreds: blindedCreds,
	}

	body, err := json.Marshal(&claimReq)
	suite.Require().NoError(err)

	req, err = http.NewRequest("POST", "/promotion/{promotionId}", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("promotionId", promotion.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler2.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code)
	suite.Assert().JSONEq(`{"message":"Error claiming promotion: wrong number of blinded tokens included","code":400}`, rr.Body.String())

	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)

	blindedCreds = make([]string, int(claim.ApproximateValue.Mul(decimal.NewFromFloat(float64(promotion.SuggestionsPerGrant)).Div(promotion.ApproximateValue)).IntPart()))
	for i := range blindedCreds {
		blindedCreds[i] = "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	}

	claimReq.BlindedCreds = blindedCreds

	body, err = json.Marshal(&claimReq)
	suite.Require().NoError(err)

	req, err = http.NewRequest("POST", "/promotion/{promotionId}", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("promotionId", promotion.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler2.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)
}

func (suite *ControllersTestSuite) TestSuggest() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	// Set a random suggestion topic each so the test suite doesn't fail when re-ran
	SetSuggestionTopic(uuid.NewV4().String() + ".grant.suggestion")

	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, _, err := kafkautils.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(context.Background(), "tcp", strings.Split(kafkaBrokers, ",")[0], "suggestion", 0)
	suite.Require().NoError(err)

	err = conn.CreateTopics(kafka.TopicConfig{Topic: suggestionTopic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	offset, err := conn.ReadLastOffset()
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
	}

	err = service.InitKafka(context.Background())
	suite.Require().NoError(err, "Failed to initialize kafka")

	promotion, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	handler := MakeSuggestion(service)

	suggestion := Suggestion{
		Type:    "oneoff-tip",
		Channel: "brave.com",
	}

	suggestionBytes, err := json.Marshal(&suggestion)
	suite.Require().NoError(err)
	suggestionPayload := base64.StdEncoding.EncodeToString(suggestionBytes)

	suggestionReq := SuggestionRequest{
		Suggestion: suggestionPayload,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(suggestionPayload)).Return(nil)

	body, err := json.Marshal(&suggestionReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          strings.Split(kafkaBrokers, ","),
		Topic:            suggestionTopic,
		Dialer:           service.kafkaDialer,
		MaxWait:          time.Second,
		RebalanceTimeout: time.Second,
		Logger:           kafka.LoggerFunc(log.Printf),
	})
	codec := service.codecs["suggestion"]

	// :cry:

	err = r.SetOffset(offset)
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	suggestionEventBinary, err := r.ReadMessage(context.Background())
	suite.Require().NoError(err)

	suggestionEvent, _, err := codec.NativeFromBinary(suggestionEventBinary.Value)
	suite.Require().NoError(err)

	suggestionEventJSON, err := codec.TextualFromNative(nil, suggestionEvent)
	suite.Require().NoError(err)

	eventMap, ok := suggestionEvent.(map[string]interface{})
	suite.Require().True(ok)
	id, ok := eventMap["id"].(string)
	suite.Require().True(ok)
	createdAt, ok := eventMap["createdAt"].(string)
	suite.Require().True(ok)

	suite.Assert().JSONEq(`{
		"id": "`+id+`",
		"createdAt": "`+createdAt+`",
		"type": "`+suggestion.Type+`",
		"channel": "`+suggestion.Channel+`",
		"totalAmount": "0.25",
		"orderId": "",
		"funding": [
			{
				"type": "ugp",
				"amount": "0.25",
				"cohort": "control",
				"promotion": "`+promotion.ID.String()+`"
			}
		]
	}`, string(suggestionEventJSON), "Incorrect suggestion event")
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
	id0 := uuid.NewV4()
	id1 := uuid.NewV4()
	id2 := uuid.NewV4()
	run := func(ids []uuid.UUID) int {
		requestPayloadStruct := ClobberedClaimsRequest{
			ClaimIDs: ids,
		}
		payload, err := json.Marshal(&requestPayloadStruct)
		suite.Require().NoError(err)
		req, err := http.NewRequest("POST", "/v1/promotions/reportclaimsummary", bytes.NewBuffer([]byte(payload)))
		suite.Require().NoError(err)

		rctx := chi.NewRouteContext()
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}
	code := run([]uuid.UUID{})
	suite.Require().Equal(http.StatusOK, code)

	code = run([]uuid.UUID{
		id0,
	})
	suite.Require().Equal(http.StatusOK, code)
	claims := []ClobberedCreds{}
	suite.Require().NoError(pg.RawDB().Select(&claims, `select * from clobbered_claims;`))
	suite.Assert().Equal(claims[0].ID, id0)

	code = run([]uuid.UUID{
		id0,
		id1,
		id2,
	})
	suite.Require().Equal(http.StatusOK, code)
	claims = []ClobberedCreds{}
	suite.Require().NoError(pg.RawDB().Select(&claims, `select * from clobbered_claims;`))
	suite.Assert().Equal(claims[0].ID, id0)
	suite.Assert().Equal(claims[1].ID, id1)
	suite.Assert().Equal(claims[2].ID, id2)
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

func (suite *ControllersTestSuite) TestClaimCompatability() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "could not connect to db")
	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "could not connect to db")
	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
	}

	now := time.Now().UTC()
	threeMonthsAgo := now.AddDate(0, -3, 0)
	later := now.Add(1000 * time.Second)
	scenarios := []struct {
		Legacy           bool      // set the claim as legacy
		Type             string    // the type of promotion (ugp/ads)
		PromoActive      bool      // set the promotion to be active
		CreatedAt        time.Time // set the created at time
		ExpiresAt        time.Time // set the expiration time
		ClaimStatus      int       // the claim request status
		ChecksReputation bool      // reputation will be checked
	}{
		{
			Legacy:           false,
			Type:             "ugp",
			PromoActive:      true,
			CreatedAt:        now,
			ExpiresAt:        later,
			ClaimStatus:      http.StatusOK,
			ChecksReputation: true,
		},
		{
			Legacy:           false,
			Type:             "ugp",
			PromoActive:      false,
			CreatedAt:        now,
			ExpiresAt:        later,
			ClaimStatus:      http.StatusGone,
			ChecksReputation: true,
		},
		{
			Legacy:           true,
			Type:             "ugp",
			PromoActive:      true,
			CreatedAt:        now,
			ExpiresAt:        later,
			ClaimStatus:      http.StatusOK,
			ChecksReputation: false,
		},
		{
			Legacy:      true,
			Type:        "ugp",
			PromoActive: false,
			CreatedAt:   now,
			ExpiresAt:   later,
			ClaimStatus: http.StatusGone,
			// these are irrelevant if claim is gone
			ChecksReputation: false,
		},
		{
			Legacy:      true,
			Type:        "ugp",
			PromoActive: false,
			CreatedAt:   now,
			ExpiresAt:   now,
			ClaimStatus: http.StatusGone,
			// these are irrelevant if claim is gone
			ChecksReputation: false,
		},
		{
			Legacy:      true,
			Type:        "ugp",
			PromoActive: true,
			CreatedAt:   now,
			ExpiresAt:   now,
			ClaimStatus: http.StatusGone,
			// these are irrelevant if claim is gone
			ChecksReputation: false,
		},
		{
			Legacy:      true,
			Type:        "ugp",
			PromoActive: true,
			CreatedAt:   threeMonthsAgo,
			ExpiresAt:   later,
			ClaimStatus: http.StatusGone,
			// these are irrelevant if claim is gone
			ChecksReputation: false,
		},
	}
	for _, test := range scenarios {
		suite.CleanDB()
		walletID := uuid.NewV4()
		publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
		suite.Require().NoError(err, "Failed to create wallet keypair")
		bat := altcurrency.BAT
		hexPublicKey := hex.EncodeToString(publicKey)
		info := walletutils.Info{
			ID:          walletID.String(),
			Provider:    "uphold",
			ProviderID:  "-",
			AltCurrency: &bat,
			PublicKey:   hexPublicKey,
			LastBalance: nil,
		}
		suite.Require().NoError(service.wallet.Datastore.UpsertWallet(context.Background(), &info), "could not insert wallet")

		blindedCreds := []string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="}
		signedCreds := []string{"hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="}
		batchProof := "hBrtClwIppLmu/qZ8EhGM1TQZUwDUosbOrVu3jMwryY="

		promotionValue := decimal.NewFromFloat(0.25)
		promotion, err := pg.CreatePromotion("ugp", 1, promotionValue, "")
		suite.Require().NoError(err, "Create promotion should succeed")

		issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: hexPublicKey}
		issuer, err = pg.InsertIssuer(issuer)
		suite.Require().NoError(err, "Insert issuer should succeed")

		suite.Require().NoError(pg.ActivatePromotion(promotion), "Activate promotion should succeed")
		_, err = pg.RawDB().Exec(
			"update promotions set expires_at = $2, created_at = $3 where id = $1",
			promotion.ID,
			test.ExpiresAt,
			test.CreatedAt,
		)
		suite.Require().NoError(err, "setting the expires_at property shouldn't fail")
		if !test.PromoActive {
			suite.Require().NoError(pg.DeactivatePromotion(promotion), "deactivating a promotion should succeed")
		}

		if test.Legacy {
			_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, promotionValue, decimal.NewFromFloat(0.0), test.Legacy)
			suite.Require().NoError(err, "an error occurred when creating a claim for wallet")
		}

		if test.PromoActive {
			if test.ClaimStatus == http.StatusOK {
				mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Any(), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
					BatchProof:   batchProof,
					SignedTokens: signedCreds,
				}, nil)
			}

			// non legacy pathway
			if test.ChecksReputation {
				mockReputation.EXPECT().
					IsWalletReputable(
						gomock.Any(),
						gomock.Any(),
						gomock.Any(),
					).
					Return(
						true,
						nil,
					)
			}
		}
		claimID := suite.ClaimGrant(
			service,
			info,
			privKey,
			promotion,
			blindedCreds,
			test.ClaimStatus,
		)
		if test.ClaimStatus == http.StatusOK && test.PromoActive {
			suite.WaitForClaimToPropagate(service, promotion, claimID)
		}
	}
}

func (suite *ControllersTestSuite) TestSuggestionMintDrain() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	walletID2 := uuid.NewV4()

	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:                     walletID.String(),
		Provider:               "uphold",
		ProviderID:             "-",
		AltCurrency:            &bat,
		PublicKey:              hex.EncodeToString(publicKey),
		LastBalance:            nil,
		UserDepositDestination: walletID2.String(),
	}
	info.UserDepositAccountProvider = new(string)
	*info.UserDepositAccountProvider = "brave"

	info2 := walletutils.Info{
		ID:          walletID2.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockReputation.EXPECT().IsWalletOnPlatform(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	tidpromotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(tidpromotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info2)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestion(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")
	err = walletDB.InsertWallet(context.Background(), &info2)
	suite.Require().NoError(err, "Failed to insert wallet")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	ctx := context.WithValue(req.Context(), appctx.BraveTransferPromotionIDCTXKey, []string{tidpromotion.ID.String()})
	req = req.WithContext(ctx)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)
	suite.Require().Equal(http.StatusOK, rr.Code)
	<-time.After(2 * time.Second)
}

func (suite *ControllersTestSuite) TestSuggestionDrainBitflyerJPYLimit() {
	// TODO: after we figure out why we are being blocked by bf enable
	//suite.T().Skip("bitflyer side unable to settle")
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	// setup bf client
	var bfClient = mockbitflyer.NewMockClient(mockCtrl)

	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)

	bfClient.EXPECT().
		FetchQuote(gomock.Any(), currencyCode, false).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         decimal.New(1, -9),
		}, nil)

	bfClient.EXPECT().
		UploadBulkPayout(
			gomock.Any(),
			gomock.Any(),
		).
		Return(&bitflyer.WithdrawToDepositIDBulkResponse{
			DryRun: false,
			Withdrawals: []bitflyer.WithdrawToDepositIDResponse{{
				CurrencyCode: BAT,
				Status:       "NOT_FOUND",
				TransferID:   "transferid",
			}},
		}, nil)

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT

	custodian := "bitflyer"

	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "brave",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		bfClient:  bfClient,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestion(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	info.UserDepositDestination = uuid.NewV4().String()
	info.UserDepositAccountProvider = &custodian

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)
	suite.Require().Equal(http.StatusOK, rr.Code)

	<-ch
	//suite.Require().True(grantAmount.Equals(altcurrency.BAT.FromProbi(tx.Probi)))

	//settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	//_, err = w.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(grantAmount), settlementAddr)
	//suite.Require().NoError(err)
}

func (suite *ControllersTestSuite) TestSuggestionDrainBitflyerNoINV() {
	// TODO: after we figure out why we are being blocked by bf enable
	//suite.T().Skip("bitflyer side unable to settle")
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	// setup bf client
	var bfClient = mockbitflyer.NewMockClient(mockCtrl)

	priceToken := uuid.NewV4()
	JPY := "JPY"
	BAT := "BAT"
	currencyCode := fmt.Sprintf("%s_%s", BAT, JPY)

	bfClient.EXPECT().
		FetchQuote(gomock.Any(), currencyCode, false).
		Return(&bitflyer.Quote{
			PriceToken:   priceToken.String(),
			ProductCode:  currencyCode,
			MainCurrency: JPY,
			SubCurrency:  BAT,
			Rate:         decimal.New(1, 1),
		}, nil)

	bfClient.EXPECT().
		UploadBulkPayout(
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, &clients.BitflyerError{ErrorIDs: []string{"NO_INV"}})

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT

	custodian := "bitflyer"

	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "brave",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		bfClient:  bfClient,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestion(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	info.UserDepositDestination = uuid.NewV4().String()
	info.UserDepositAccountProvider = &custodian

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)
	suite.Require().Equal(http.StatusOK, rr.Code)

	<-time.After(2 * time.Second)

	var drainJob = getClaimDrainEntry(pg.(*DatastoreWithPrometheus).base.(*Postgres))
	suite.Require().True(drainJob.Erred)
	suite.Require().Equal(*drainJob.ErrCode, "NO_INV", "error code should be no inv")

}

func (suite *ControllersTestSuite) TestSuggestionDrainBitflyer() {
	// TODO: after we figure out why we are being blocked by bf enable
	suite.T().Skip("bitflyer side unable to settle")
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	// setup bf client
	bfClient, err := bitflyer.New()
	suite.Require().NoError(err)

	// setup token in client
	payload := bitflyer.TokenPayload{
		GrantType:         "client_credentials",
		ClientID:          os.Getenv("BITFLYER_CLIENT_ID"),
		ClientSecret:      os.Getenv("BITFLYER_CLIENT_SECRET"),
		ExtraClientSecret: os.Getenv("BITFLYER_EXTRA_CLIENT_SECRET"),
	}
	auth, err := bfClient.RefreshToken(
		context.Background(),
		payload,
	)
	suite.Require().NoError(err)
	bfClient.SetAuthToken(auth.AccessToken)

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT

	custodian := "bitflyer"

	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "brave",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		bfClient:  bfClient,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestion(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	info.UserDepositDestination = uuid.NewV4().String()
	info.UserDepositAccountProvider = &custodian

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)
	suite.Require().Equal(http.StatusOK, rr.Code)

	<-ch

	//suite.Require().True(grantAmount.Equals(altcurrency.BAT.FromProbi(tx.Probi)))

	//settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	//_, err = w.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(grantAmount), settlementAddr)
	//suite.Require().NoError(err)
}

func (suite *ControllersTestSuite) TestSuggestionDrainV2() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	w := uphold.Wallet{
		Info:    info,
		PrivKey: privKey,
		PubKey:  publicKey,
	}
	err = w.Register("drain-card-test")
	suite.Require().NoError(err, "Failed to register wallet")

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	err = service.InitHotWallet(context.Background())
	suite.Require().NoError(err, "Failed to init hot wallet")

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestionV2(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &w.Info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	info.UserDepositDestination = w.ProviderID
	info.UserDepositAccountProvider = new(string)
	*info.UserDepositAccountProvider = "uphold"

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)

	var dsv2r = new(DrainSuggestionV2Response)

	err = json.NewDecoder(rr.Result().Body).Decode(dsv2r)

	suite.Require().NoError(err, "Failed to unmarshal response")

	suite.Require().Equal(http.StatusOK, rr.Code)

	tx := <-ch
	suite.Require().True(grantAmount.Equals(altcurrency.BAT.FromProbi(tx.Probi)))

	settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	_, err = w.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(grantAmount), settlementAddr)
	suite.Require().NoError(err)

	// pull out drain id, and check the datastore has completed state
	drainPoll, err := service.Datastore.GetDrainPoll(dsv2r.DrainID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "complete")

	// test other conditions
	delayedID := uuid.NewV4()
	pendingID := uuid.NewV4()
	inprogressID := uuid.NewV4()

	err = claimDrainFixtures(pg.RawDB(), delayedID, walletID, false, true)
	suite.Require().NoError(err, "failed to fixture claim_drain")
	err = claimDrainFixtures(pg.RawDB(), pendingID, walletID, false, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")
	err = claimDrainFixtures(pg.RawDB(), inprogressID, walletID, true, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")
	err = claimDrainFixtures(pg.RawDB(), inprogressID, walletID, false, false)
	suite.Require().NoError(err, "failed to fixture claim_drain")

	// there is an error in one of the drainings
	drainPoll, err = service.Datastore.GetDrainPoll(&delayedID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "delayed")

	// at least one drain poll batch has started, but all are not complete
	drainPoll, err = service.Datastore.GetDrainPoll(&pendingID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "pending")

	// none of the drain poll batches have started
	drainPoll, err = service.Datastore.GetDrainPoll(&inprogressID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "in_progress")

	// unknown batch_id
	unknownID := uuid.NewV4()
	drainPoll, err = service.Datastore.GetDrainPoll(&unknownID)
	suite.Require().NoError(err, "Failed to get drain poll response")

	suite.Require().True(drainPoll.Status == "unknown")

}

func claimDrainFixtures(db *sqlx.DB, batchID, walletID uuid.UUID, completed, erred bool) error {
	_, err := db.Exec(`INSERT INTO claim_drain (batch_id, credentials, completed, erred, wallet_id, total) values ($1, '[{"t":123}]', $2, $3, $4, $5);`, batchID, completed, erred, walletID, 1)
	return err
}

func (suite *ControllersTestSuite) TestSuggestionDrain() {
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *walletutils.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	w := uphold.Wallet{
		Info:    info,
		PrivKey: privKey,
		PubKey:  publicKey,
	}
	err = w.Register("drain-card-test")
	suite.Require().NoError(err, "Failed to register wallet")

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	err = service.InitHotWallet(context.Background())
	suite.Require().NoError(err, "Failed to init hot wallet")

	promotion, err := service.Datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = service.wallet.Datastore.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.Datastore.CreateClaim(promotion.ID, info.ID, grantAmount, decimal.NewFromFloat(claimBonus), false)
	suite.Require().NoError(err, "create a claim for a promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(walletID.String())).Return(nil)

	handler := middleware.HTTPSignedOnly(service)(DrainSuggestion(service))

	drainReq := DrainSuggestionRequest{
		WalletID: walletID,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	body, err := json.Marshal(&drainReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = info.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	err = walletDB.InsertWallet(context.Background(), &w.Info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	info.UserDepositDestination = w.ProviderID
	info.UserDepositAccountProvider = new(string)
	*info.UserDepositAccountProvider = "uphold"

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	b, _ := httputil.DumpResponse(rr.Result(), true)
	fmt.Printf("%s", b)
	suite.Require().Equal(http.StatusOK, rr.Code)

	tx := <-ch
	suite.Require().True(grantAmount.Equals(altcurrency.BAT.FromProbi(tx.Probi)))

	settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	_, err = w.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(grantAmount), settlementAddr)
	suite.Require().NoError(err)
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

func (suite *ControllersTestSuite) TestBraveFundsTransaction() {
	// Set a random suggestion topic each so the test suite doesn't fail when re-ran
	SetSuggestionTopic(uuid.NewV4().String() + ".grant.suggestion")
	pg, _, err := NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres   ")
	walletDB, _, err := wallet.NewPostgres()
	suite.Require().NoError(err, "Failed to get postgres conn")

	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, _, err := kafkautils.TLSDialer()
	suite.Require().NoError(err)
	conn, err := dialer.DialLeader(context.Background(), "tcp", strings.Split(kafkaBrokers, ",")[0], "suggestion", 0)
	suite.Require().NoError(err)

	err = conn.CreateTopics(kafka.TopicConfig{Topic: suggestionTopic, NumPartitions: 1, ReplicationFactor: 1})
	suite.Require().NoError(err)

	offset, err := conn.ReadLastOffset()
	suite.Require().NoError(err)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	info := walletutils.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}

	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockReputation.EXPECT().IsWalletReputable(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)
	err = walletDB.InsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		Datastore: pg,
		cbClient:  mockCB,
		wallet: &wallet.Service{
			Datastore: walletDB,
		},
		reputationClient: mockReputation,
	}

	err = service.InitKafka(context.Background())
	suite.Require().NoError(err, "Failed to initialize kafka")

	promotion, err := service.Datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.Datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	issuerName := promotion.ID.String() + ":control"
	issuerPublicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	blindedCreds := []string{"XhBPMjh4vMw+yoNjE7C5OtoTz2rCtfuOXO/Vk7UwWzY="}
	signedCreds := []string{"NJnOyyL6YAKMYo6kSAuvtG+/04zK1VNaD9KdKwuzAjU="}
	proof := "IiKqfk10e7SJ54Ud/8FnCf+sLYQzS4WiVtYAM5+RVgApY6B9x4CVbMEngkDifEBRD6szEqnNlc3KA8wokGV5Cw=="
	sig := "PsavkSWaqsTzZjmoDBmSu6YxQ7NZVrs2G8DQ+LkW5xOejRF6whTiuUJhr9dJ1KlA+79MDbFeex38X5KlnLzvJw=="
	preimage := "125KIuuwtHGEl35cb5q1OLSVepoDTgxfsvwTc7chSYUM2Zr80COP19EuMpRQFju1YISHlnB04XJzZYN2ieT9Ng=="

	mockCB.EXPECT().CreateIssuer(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(defaultMaxTokensPerIssuer)).Return(nil)
	mockCB.EXPECT().GetIssuer(gomock.Any(), gomock.Eq(issuerName)).Return(&cbr.IssuerResponse{
		Name:      issuerName,
		PublicKey: issuerPublicKey,
	}, nil)
	mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Eq(issuerName), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
		BatchProof:   proof,
		SignedTokens: signedCreds,
	}, nil)

	err = walletDB.UpsertWallet(context.Background(), &info)
	suite.Require().NoError(err, "Failed to insert wallet")

	claimID := suite.ClaimGrant(service, info, privKey, promotion, blindedCreds, http.StatusOK)
	suite.WaitForClaimToPropagate(service, promotion, claimID)

	handler := MakeSuggestion(service)

	orderID, err := suite.CreateOrder()
	suite.Require().NoError(err)
	validOrderID := uuid.Must(uuid.FromString(orderID))

	orderPending, err := service.Datastore.GetOrder(validOrderID)
	suite.Require().NoError(err)
	suite.Require().Equal("pending", orderPending.Status)

	suggestion := Suggestion{
		Type:    "payment",
		OrderID: &validOrderID,
		Channel: "brave.com",
	}

	suggestionBytes, err := json.Marshal(&suggestion)
	suite.Require().NoError(err)
	suggestionPayload := base64.StdEncoding.EncodeToString(suggestionBytes)

	suggestionReq := SuggestionRequest{
		Suggestion: suggestionPayload,
		Credentials: []CredentialBinding{{
			PublicKey:     issuerPublicKey,
			Signature:     sig,
			TokenPreimage: preimage,
		}},
	}

	mockCB.EXPECT().RedeemCredentials(gomock.Any(), gomock.Eq([]cbr.CredentialRedemption{{
		Issuer:        issuerName,
		TokenPreimage: preimage,
		Signature:     sig,
	}}), gomock.Eq(suggestionPayload)).Return(nil)

	body, err := json.Marshal(&suggestionReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/suggestion", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:          strings.Split(kafkaBrokers, ","),
		Topic:            suggestionTopic,
		Dialer:           service.kafkaDialer,
		MaxWait:          time.Second,
		RebalanceTimeout: time.Second,
		Logger:           kafka.LoggerFunc(log.Printf),
	})

	// :cry:
	err = r.SetOffset(offset)
	suite.Require().NoError(err)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	codec := service.codecs["suggestion"]

	suggestionEventBinary, err := r.ReadMessage(context.Background())
	suite.Require().NoError(err)

	suggestionEvent, _, err := codec.NativeFromBinary(suggestionEventBinary.Value)
	suite.Require().NoError(err)
	suite.Require().NotNil(suggestionEvent)

	for {
		updatedOrder, err := service.Datastore.GetOrder(validOrderID)
		suite.Require().NoError(err)
		if updatedOrder.Status == "paid" {
			break
		}
		<-time.After(10 * time.Millisecond)
	}
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
