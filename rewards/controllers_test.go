package rewards

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"

	"github.com/brave-intl/bat-go/utils/clients/ratios"
	ratiosmock "github.com/brave-intl/bat-go/utils/clients/ratios/mock"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
)

type RewardsControllersTestSuite struct {
	suite.Suite
}

func TestRewardsControllersTestSuite(t *testing.T) {
	suite.Run(t, new(RewardsControllersTestSuite))
}

func (suite *RewardsControllersTestSuite) TestGetParametersController() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockRatios := ratiosmock.NewMockClient(mockCtrl)
	mockRatios.EXPECT().FetchRate(gomock.Any(), gomock.Eq("BAT"), gomock.Eq("USD")).
		Return(&ratios.RateResponse{
			Payload: map[string]decimal.Decimal{
				"USD": decimal.Zero,
				"BAT": decimal.Zero,
			}}, nil)

	var (
		h      = GetParametersHandler(NewService(context.Background(), mockRatios))
		params = new(ParametersV1)
	)

	req, err := http.NewRequest("GET", "/v1/parameters", nil)
	suite.Require().NoError(err, "failed to make new request")

	rctx := chi.NewRouteContext()
	r := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	suite.Require().Equal(
		http.StatusOK,
		rr.Code,
		"was expecting an ok response",
	)

	suite.Require().NoError(
		json.Unmarshal(rr.Body.Bytes(), &params),
		"should be no error with unmarshalling response",
	)
	suite.Require().Equal(
		float64(0),
		params.BATRate,
		"was expecting 0 for the bat rate",
	)
	suite.Require().Greater(
		len(params.AutoContribute.Choices),
		0,
		"was expecting more than one ac choices",
	)
}

func (suite *RewardsControllersTestSuite) TestGetParametersControllerV2() {
	mockCtrl := gomock.NewController(suite.T())
	defer mockCtrl.Finish()
	mockRatios := ratiosmock.NewMockClient(mockCtrl)
	now := time.Now()
	mockRatios.EXPECT().FetchRate(
		gomock.Any(),
		gomock.Eq("BAT"),
		gomock.Eq("USD"),
	).
		Return(&ratios.RateResponse{
			LastUpdated: now,
			Payload: map[string]decimal.Decimal{
				"BAT": decimal.NewFromFloat(1),
			},
		}, nil)

	var (
		params = new(ParametersV2)
		h      = GetParametersHandlerV2(
			NewService(
				context.Background(),
				mockRatios,
			),
		)
	)

	req, err := http.NewRequest("GET", "/v2/parameters", nil)
	suite.Require().NoError(err, "failed to make new request")

	rctx := chi.NewRouteContext()
	r := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	suite.Require().Equal(
		http.StatusOK,
		rr.Code,
		"was expecting an ok response",
	)
	suite.Require().NoError(
		json.Unmarshal(rr.Body.Bytes(), &params),
		"should be no error with unmarshalling response",
	)
	suite.Require().NotEqual(
		now,
		params.Rate.LastUpdated.String(),
		"was expecting non 0 for the bat rate",
	)
	suite.Require().NotEqual(
		decimal.Zero,
		params.Rate.Payload["BAT"],
		"was expecting non 0 for the bat rate",
	)
	suite.Require().Greater(
		len(params.AutoContribute.Choices),
		0,
		"was expecting more than one ac choices",
	)
}
