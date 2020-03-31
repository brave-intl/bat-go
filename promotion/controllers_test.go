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
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	mockbalance "github.com/brave-intl/bat-go/utils/clients/balance/mock"
	cbr "github.com/brave-intl/bat-go/utils/clients/cbr"
	mockcb "github.com/brave-intl/bat-go/utils/clients/cbr/mock"
	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	mockreputation "github.com/brave-intl/bat-go/utils/clients/reputation/mock"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/brave-intl/bat-go/wallet/provider/uphold"
	walletservice "github.com/brave-intl/bat-go/wallet/service"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
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
	pg, err := NewPostgres("", false)
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
	tables := []string{"claim_creds", "claims", "wallets", "issuers", "promotions"}

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.DB.Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *ControllersTestSuite) TestGetPromotions() {

	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	cbClient, err := cbr.New()
	suite.Require().NoError(err, "Failed to create challenge bypass client")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	walletID := uuid.NewV4()
	wallet := wallet.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: nil,
		PublicKey:   "-",
		LastBalance: nil,
	}

	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	service := &Service{
		datastore: pg,
		cbClient:  cbClient,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
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
			"publicKeys" : [],
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

	promotionGeneric, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Failed to create a general promotion")

	promotionDesktop, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(20.0), "desktop")
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

	err = service.datastore.ActivatePromotion(promotionGeneric)
	suite.Require().NoError(err, "Failed to activate promotion")
	err = service.datastore.ActivatePromotion(promotionDesktop)
	suite.Require().NoError(err, "Failed to activate promotion")

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
	_, err = pg.DB.Exec(statement, promotionDesktop.ID, wallet.ID, promotionDesktop.ApproximateValue)
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

func (suite *ControllersTestSuite) ClaimGrant(service *Service, wallet wallet.Info, privKey crypto.Signer, promotion *Promotion, blindedCreds []string) GetClaimResponse {
	handler := middleware.HTTPSignedOnly(service)(ClaimPromotion(service))

	walletID, err := uuid.FromString(wallet.ID)
	suite.Require().NoError(err)

	claimReq := ClaimRequest{
		WalletID:     walletID,
		BlindedCreds: blindedCreds,
	}

	body, err := json.Marshal(&claimReq)
	suite.Require().NoError(err)

	req, err := http.NewRequest("POST", "/promotion/{promotionId}", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = wallet.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("promotionId", promotion.ID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	var claimResp ClaimResponse
	err = json.Unmarshal(rr.Body.Bytes(), &claimResp)
	suite.Require().NoError(err)

	handler = GetClaim(service)

	req, err = http.NewRequest("GET", "/promotion/{promotionId}/claims/{claimId}", nil)
	suite.Require().NoError(err)

	ctx, _ := context.WithTimeout(req.Context(), 500*time.Millisecond)
	rctx.URLParams.Add("claimId", claimResp.ClaimID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
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

	return getClaimResp
}

func (suite *ControllersTestSuite) TestClaimGrant() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	cbClient, err := cbr.New()
	suite.Require().NoError(err, "Failed to create challenge bypass client")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	wallet := wallet.Info{
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
	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	service := &Service{
		datastore: pg,
		cbClient:  cbClient,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
		},
		reputationClient: mockReputation,
	}

	promotion, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	blindedCreds := make([]string, promotion.SuggestionsPerGrant)
	for i := range blindedCreds {
		blindedCreds[i] = "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	}

	_ = suite.ClaimGrant(service, wallet, privKey, promotion, blindedCreds)

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

	promotion, _, claim := suite.setupAdsClaim(service, &wallet, 0)

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
	s.KeyID = wallet.ID
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
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	// Set a random suggestion topic each so the test suite doesn't fail when re-ran
	SetSuggestionTopic(uuid.NewV4().String() + ".grant.suggestion")

	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, err := tlsDialer()
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
	wallet := wallet.Info{
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
	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		datastore: pg,
		cbClient:  mockCB,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
		},
		reputationClient: mockReputation,
	}

	err = service.InitKafka()
	suite.Require().NoError(err, "Failed to initialize kafka")

	promotion, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.datastore.ActivatePromotion(promotion)
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

	suite.ClaimGrant(service, wallet, privKey, promotion, blindedCreds)

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
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	service := &Service{
		datastore: pg,
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
	w := &wallet.Info{
		ID:         walletID,
		Provider:   "uphold",
		ProviderID: uuid.NewV4().String(),
		PublicKey:  publicKey,
	}
	err = pg.UpsertWallet(w)
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
	promotion, issuer, claim := suite.setupAdsClaim(service, w, 0)

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "apply claim to wallet")

	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Require().Equal(http.StatusOK, code)
	suite.Assert().JSONEq(`{
		"earnings": "30",
		"lastClaim": "`+claim.CreatedAt.Format(time.RFC3339Nano)+`",
		"type": "ads"
	}`, body, "expected a aggregated claim response")

	// not ignored bonus promotion
	promotion, issuer, claim = suite.setupAdsClaim(service, w, 20)

	_, err = pg.ClaimForWallet(promotion, issuer, w, blindedCreds)
	suite.Require().NoError(err, "apply claim to wallet")

	body, code = suite.checkGetClaimSummary(service, walletID, "ads")
	suite.Require().Equal(http.StatusOK, code)
	suite.Assert().JSONEq(`{
		"earnings": "40",
		"lastClaim": "`+claim.CreatedAt.Format(time.RFC3339Nano)+`",
		"type": "ads"
	}`, body, "expected a aggregated claim response")
}

func (suite *ControllersTestSuite) setupAdsClaim(service *Service, w *wallet.Info, claimBonus float64) (*Promotion, *Issuer, *Claim) {
	// promo amount can be different than individual grant amount
	promoAmount := decimal.NewFromFloat(25.0)
	promotion, err := service.datastore.CreatePromotion("ads", 2, promoAmount, "")
	suite.Require().NoError(err, "a promotion could not be created")

	publicKey := "dHuiBIasUO0khhXsWgygqpVasZhtQraDSZxzJW2FKQ4="
	issuer := &Issuer{PromotionID: promotion.ID, Cohort: "control", PublicKey: publicKey}
	issuer, err = service.datastore.InsertIssuer(issuer)
	suite.Require().NoError(err, "Insert issuer should succeed")

	err = service.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "a promotion should be activated")

	grantAmount := decimal.NewFromFloat(30.0)
	claim, err := service.datastore.CreateClaim(promotion.ID, w.ID, grantAmount, decimal.NewFromFloat(claimBonus))
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
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	mockLedger := mockledger.NewMockClient(mockCtrl)

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		datastore: pg,
		cbClient:  mockCB,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
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
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "could not connect to db")
	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)
	mockBalance := mockbalance.NewMockClient(mockCtrl)

	service := &Service{
		datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
		balanceClient:    mockBalance,
	}
	handler := PostReportClobberedClaims(service)
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
	suite.Require().NoError(pg.DB.Select(&claims, `select * from clobbered_claims;`))
	suite.Assert().Equal(claims[0].ID, id0)

	code = run([]uuid.UUID{
		id0,
		id1,
		id2,
	})
	suite.Require().Equal(http.StatusOK, code)
	claims = []ClobberedCreds{}
	suite.Require().NoError(pg.DB.Select(&claims, `select * from clobbered_claims;`))
	suite.Assert().Equal(claims[0].ID, id0)
	suite.Assert().Equal(claims[1].ID, id1)
	suite.Assert().Equal(claims[2].ID, id2)
}

func (suite *ControllersTestSuite) TestClaimCompatability() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "could not connect to db")
	mockReputation := mockreputation.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)
	mockBalance := mockbalance.NewMockClient(mockCtrl)

	service := &Service{
		datastore:        pg,
		reputationClient: mockReputation,
		cbClient:         mockCB,
		balanceClient:    mockBalance,
		wallet: walletservice.Service{
			Datastore: pg,
		},
	}

	scenarios := []struct {
		Legacy             bool
		Redeemed           bool
		ChecksReputation   bool
		InvalidatesBalance bool
		Type               string
	}{
		{
			Legacy:             false,
			Redeemed:           false,
			ChecksReputation:   true,
			InvalidatesBalance: false,
			Type:               "ugp",
		},
		{
			Legacy:             false,
			Redeemed:           true,
			ChecksReputation:   false,
			InvalidatesBalance: false,
			Type:               "ugp",
		},
		{
			Legacy:             true,
			Redeemed:           false,
			ChecksReputation:   false,
			InvalidatesBalance: true,
			Type:               "ugp",
		},
		{
			Legacy:             true,
			Redeemed:           true,
			ChecksReputation:   false,
			InvalidatesBalance: false,
			Type:               "ugp",
		},
	}
	for _, test := range scenarios {
		walletID := uuid.NewV4()
		publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
		suite.Require().NoError(err, "Failed to create wallet keypair")
		bat := altcurrency.BAT
		hexPublicKey := hex.EncodeToString(publicKey)
		w := &wallet.Info{
			ID:          walletID.String(),
			Provider:    "uphold",
			ProviderID:  "-",
			AltCurrency: &bat,
			PublicKey:   hexPublicKey,
			LastBalance: nil,
		}
		suite.Require().NoError(pg.UpsertWallet(w), "could not insert wallet")

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

		var claim *Claim
		if test.Legacy {
			claim, err = service.datastore.CreateClaim(promotion.ID, w.ID, promotionValue, decimal.NewFromFloat(0.0))
			suite.Require().NoError(err, "an error occurred when creating a claim for wallet")
			_, err = pg.DB.Exec(`update claims set legacy_claimed = $2 where id = $1`, claim.ID.String(), test.Legacy)
			suite.Require().NoError(err, "an error occurred when setting legacy or redeemed")
		}

		mockCB.EXPECT().SignCredentials(gomock.Any(), gomock.Any(), gomock.Eq(blindedCreds)).Return(&cbr.CredentialsIssueResponse{
			BatchProof:   batchProof,
			SignedTokens: signedCreds,
		}, nil)

		if test.Redeemed {
			// if redeemed, the mockCB's SignCredentials is used up here
			// otherwise used up in suite.ClaimGrant below
			if !test.Legacy {
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
			} else {
				mockBalance.EXPECT().
					InvalidateBalance(
						gomock.Any(),
						gomock.Eq(walletID),
					).
					Return(nil)
			}
			_ = suite.ClaimGrant(service, *w, privKey, promotion, blindedCreds)
		}

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
		if test.InvalidatesBalance {
			mockBalance.EXPECT().
				InvalidateBalance(
					gomock.Any(),
					gomock.Eq(walletID),
				).
				Return(nil)
		}

		// if NOT redeemed, the mockCB's SignCredentials will be used up here
		_ = suite.ClaimGrant(service, *w, privKey, promotion, blindedCreds)
	}
}

func (suite *ControllersTestSuite) TestSuggestionDrain() {
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	ch := make(chan *wallet.TransactionInfo)

	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()

	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err, "Failed to create wallet keypair")

	walletID := uuid.NewV4()
	bat := altcurrency.BAT
	wallet := wallet.Info{
		ID:          walletID.String(),
		Provider:    "uphold",
		ProviderID:  "-",
		AltCurrency: &bat,
		PublicKey:   hex.EncodeToString(publicKey),
		LastBalance: nil,
	}
	wal := uphold.Wallet{
		Info:    wallet,
		PrivKey: privKey,
		PubKey:  publicKey,
	}
	err = wal.Register("drain-card-test")
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
	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		datastore: pg,
		cbClient:  mockCB,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
		},
		reputationClient: mockReputation,
		drainChannel:     ch,
	}

	err = service.InitHotWallet()
	suite.Require().NoError(err, "Failed to init hot wallet")

	promotion, err := service.datastore.CreatePromotion("ads", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	err = pg.UpsertWallet(&wallet)
	suite.Require().NoError(err, "the wallet failed to be inserted")

	claimBonus := 0.25
	grantAmount := decimal.NewFromFloat(0.25)
	_, err = service.datastore.CreateClaim(promotion.ID, wallet.ID, grantAmount, decimal.NewFromFloat(claimBonus))
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

	suite.ClaimGrant(service, wallet, privKey, promotion, blindedCreds)

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
	s.KeyID = wallet.ID
	s.Headers = []string{"digest", "(request-target)"}

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusBadRequest, rr.Code, "Wallet without payout address should fail")

	req, err = http.NewRequest("POST", "/suggestion/drain", bytes.NewBuffer(body))
	suite.Require().NoError(err)

	err = s.Sign(privKey, crypto.Hash(0), req)
	suite.Require().NoError(err)

	payoutAddress := wal.ProviderID
	wallet.PayoutAddress = &payoutAddress
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Require().Equal(http.StatusOK, rr.Code)

	tx := <-ch
	suite.Require().True(grantAmount.Equals(altcurrency.BAT.FromProbi(tx.Probi)))

	settlementAddr := os.Getenv("BAT_SETTLEMENT_ADDRESS")
	_, err = wal.Transfer(altcurrency.BAT, altcurrency.BAT.ToProbi(grantAmount), settlementAddr)
	suite.Require().NoError(err)
}

// THIS CODE IS A QUICK AND DIRTY HACK
// WE SHOULD DELETE ALL OF THIS AND MOVE OVER TO THE PAYMENT SERVICE ONCE DEMO IS DONE.

// CreateOrder creates orders given the total price, merchant ID, status and items of the order
func (suite *ControllersTestSuite) CreateOrder() (string, error) {
	pg, err := NewPostgres("", false)
	tx := pg.DB.MustBegin()

	var id string

	err = tx.Get(&id, `
			INSERT INTO orders (total_price, merchant_id, status, currency)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, 0.25, "brave.com", "pending", "BAT")

	err = tx.Commit()
	if err != nil {
		_ = tx.Rollback()
		return "", err
	}

	return id, nil
}

func (suite *ControllersTestSuite) TestBraveFundsTransaction() {
	// Set a random suggestion topic each so the test suite doesn't fail when re-ran
	SetSuggestionTopic(uuid.NewV4().String() + ".grant.suggestion")
	pg, err := NewPostgres("", false)
	suite.Require().NoError(err, "Failed to get postgres conn")

	// FIXME stick kafka setup in suite setup
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")

	dialer, err := tlsDialer()
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
	wallet := wallet.Info{
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
	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	mockCB := mockcb.NewMockClient(mockCtrl)

	service := &Service{
		datastore: pg,
		cbClient:  mockCB,
		wallet: walletservice.Service{
			Datastore:    pg,
			LedgerClient: mockLedger,
		},
		reputationClient: mockReputation,
	}

	err = service.InitKafka()
	suite.Require().NoError(err, "Failed to initialize kafka")

	promotion, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(0.25), "")
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.datastore.ActivatePromotion(promotion)
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

	suite.ClaimGrant(service, wallet, privKey, promotion, blindedCreds)

	handler := MakeSuggestion(service)

	orderID, err := suite.CreateOrder()
	suite.Require().NoError(err)
	validOrderID := uuid.Must(uuid.FromString(orderID))

	orderPending, err := service.datastore.GetOrder(validOrderID)
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

	updatedOrder, err := service.datastore.GetOrder(validOrderID)
	suite.Require().NoError(err)
	suite.Assert().Equal("paid", updatedOrder.Status)
}
