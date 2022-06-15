package rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	mockRatios.EXPECT().FetchRate(gomock.Any(), gomock.Eq("bat"), gomock.Eq("usd")).
		Return(&ratios.RateResponse{
			Payload: map[string]decimal.Decimal{
				"usd": decimal.Zero,
				"bat": decimal.Zero,
			}}, nil)

	var (
		s      = NewService(context.Background(), mockRatios)
		hGet   = GetParametersHandler(s)
		hPatch = SetPayoutStatusHandler(s)
		params = new(ParametersV1)
	)

	fmt.Println("s.payoutStatus: ", s.payoutStatus)
	rctx := chi.NewRouteContext()

	sporeq, err := http.NewRequest("PATCH", "/parameters/payoutStatus", bytes.NewBufferString(`
{"uphold":"processing","gemini":"off","bitflyer":"off","unverified":"off"}
	`))
	if err != nil {
		t.Error("failed to make new request: ", err)
	}

	r := sporeq.WithContext(context.WithValue(sporeq.Context(), chi.RouteCtxKey, rctx))

	rrPatch := httptest.NewRecorder()
	hPatch.ServeHTTP(rrPatch, r)
	if rrPatch.Code != http.StatusOK {
		t.Log("result: ", rrPatch.Body.String())
		t.Error("was expecting an ok response: ", rrPatch.Code)
	}

	req, err := http.NewRequest("GET", "/parameters", nil)
	if err != nil {
		t.Error("failed to make new request: ", err)
	}

	r = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rrGet := httptest.NewRecorder()
	hGet.ServeHTTP(rrGet, r)
	if rrGet.Code != http.StatusOK {
		t.Log("result: ", rrGet.Body.String())
		t.Error("was expecting an ok response: ", rrGet.Code)
	}

	if err = json.Unmarshal(rrGet.Body.Bytes(), &params); err != nil {
		t.Error("should be no error with unmarshalling response: ", err)
	}

	if params.BATRate != 0 {
		t.Error("was expecting 0 for the bat rate")
	}
	if params.PayoutStatus.Uphold != "processing" {
		t.Error("was expecting uphold to be set to processing")
	}
	if len(params.AutoContribute.Choices) == 0 {
		t.Error("was expecting more than one ac choices")
	}
}
