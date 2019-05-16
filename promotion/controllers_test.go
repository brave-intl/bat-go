// +build integration

package promotion

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/middleware"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	mockledger "github.com/brave-intl/bat-go/utils/ledger/mock"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type ControllersTestSuite struct {
	suite.Suite
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
		datastore:    pg,
		cbClient:     cbClient,
		ledgerClient: mockLedger,
	}
	handler := GetAvailablePromotions(service)

	req, err := http.NewRequest("GET", "/promotions?paymentId="+walletID.String(), nil)
	suite.Require().NoError(err, "Failed to create get promotions request")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	suite.Assert().Equal(http.StatusOK, rr.Code)
	suite.Assert().JSONEq(`{"promotions": []}`, rr.Body.String(), "unexpected result")

	promotion, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0))
	suite.Require().NoError(err, "Failed to create promotion")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusOK, rr.Code)
	expected := `{
    "promotions": [
        {
            "approximateValue": "15",
            "available": false,
            "createdAt": "` + promotion.CreatedAt.Format(time.RFC3339Nano) + `",
            "expiresAt": "` + promotion.ExpiresAt.Format(time.RFC3339Nano) + `",
            "id": "` + promotion.ID.String() + `",
            "suggestionsPerGrant": 60,
            "type": "ugp",
            "version": 5
        }
    ]
	}`
	suite.Assert().JSONEq(expected, rr.Body.String(), "unexpected result")

	err = service.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	suite.Assert().Equal(http.StatusOK, rr.Code)
	expected = `{
    "promotions": [
        {
            "approximateValue": "15",
            "available": true,
            "createdAt": "` + promotion.CreatedAt.Format(time.RFC3339Nano) + `",
            "expiresAt": "` + promotion.ExpiresAt.Format(time.RFC3339Nano) + `",
            "id": "` + promotion.ID.String() + `",
            "suggestionsPerGrant": 60,
            "type": "ugp",
            "version": 5
        }
    ]
	}`
	suite.Assert().JSONEq(expected, rr.Body.String(), "unexpected result")
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

	mockLedger := mockledger.NewMockClient(mockCtrl)
	mockLedger.EXPECT().GetWallet(gomock.Any(), gomock.Eq(walletID)).Return(&wallet, nil)

	service := &Service{
		datastore:    pg,
		cbClient:     cbClient,
		ledgerClient: mockLedger,
	}
	handler := middleware.HTTPSignedOnly(service)(ClaimPromotion(service))

	promotion, err := service.datastore.CreatePromotion("ugp", 2, decimal.NewFromFloat(15.0))
	suite.Require().NoError(err, "Failed to create promotion")
	err = service.datastore.ActivatePromotion(promotion)
	suite.Require().NoError(err, "Failed to activate promotion")

	claimReq := ClaimRequest{
		PaymentID:    walletID,
		BlindedCreds: make([]string, promotion.SuggestionsPerGrant),
	}

	for i := range claimReq.BlindedCreds {
		claimReq.BlindedCreds[i] = "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
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
	suite.Assert().Equal(http.StatusOK, rr.Code)

	var claimResp ClaimResponse
	err = json.Unmarshal(rr.Body.Bytes(), &claimResp)
	suite.Assert().NoError(err)

	handler = GetClaim(service)

	req, err = http.NewRequest("GET", "/promotion/{promotionId}/claims/{claimId}", nil)
	suite.Require().NoError(err)

	ctx, _ := context.WithTimeout(req.Context(), 500*time.Millisecond)
	rctx.URLParams.Add("claimId", claimResp.ClaimID.String())
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	for rr.Code != http.StatusOK {
		select {
		case <-ctx.Done():
			break
		default:
			time.Sleep(50 * time.Millisecond)
			rr = httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
		}
	}
	suite.Assert().Equal(http.StatusOK, rr.Code, "Async signing timed out")

	var getClaimResp GetClaimResponse
	err = json.Unmarshal(rr.Body.Bytes(), &getClaimResp)
	suite.Assert().NoError(err)

	suite.Assert().Equal(promotion.SuggestionsPerGrant, len(getClaimResp.SignedCreds), "Signed credentials should have the same length")
}

func TestControllersTestSuite(t *testing.T) {
	suite.Run(t, new(ControllersTestSuite))
}
