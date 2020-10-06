package rewards

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/utils/clients/ratios"
	ratiosmock "github.com/brave-intl/bat-go/utils/clients/ratios/mock"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
)

func TestGetParametersController(t *testing.T) {
	mockCtrl := gomock.NewController(t)
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

	req, err := http.NewRequest("GET", "/parameters", nil)
	if err != nil {
		t.Error("failed to make new request: ", err)
	}

	rctx := chi.NewRouteContext()
	r := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusOK {
		t.Log("result: ", rr.Body.String())
		t.Error("was expecting an ok response: ", rr.Code)
	}

	if err = json.Unmarshal(rr.Body.Bytes(), &params); err != nil {
		t.Error("should be no error with unmarshalling response: ", err)
	}

	if params.BATRate != 0 {
		t.Error("was expecting 0 for the bat rate")
	}
	if len(params.AutoContribute.Choices) == 0 {
		t.Error("was expecting more than one ac choices")
	}
}
