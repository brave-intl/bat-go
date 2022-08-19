package rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/shopspring/decimal"

	"github.com/brave-intl/bat-go/libs/clients/ratios"
	ratiosmock "github.com/brave-intl/bat-go/libs/clients/ratios/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
)

type mockGetObjectAPI func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)

func (m mockGetObjectAPI) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return m(ctx, params, optFns...)
}

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

	var mockS3PayoutStatus = mockGetObjectAPI(func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
		return &s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewBufferString(`{
				"uphold":"processing",
				"gemini":"off",
				"bitflyer":"off",
				"unverified":"off"
			}
			`)),
		}, nil
	})

	var mockS3CustodianRegions = mockGetObjectAPI(func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
		return &s3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewBufferString(`{
				"uphold": {
					"allow": [],
					"block": []
				},
				"gemini": {
					"allow": [],
					"block": []
				},
				"bitflyer": {
					"allow": [],
					"block": []
				}
			}`)),
		}, nil
	})

	var mockS3 = mockGetObjectAPI(func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
		if *params.Key == "payout-status.json" {
			return mockS3PayoutStatus(ctx, params, optFns...)
		} else if *params.Key == "custodian-regions.json" {
			return mockS3CustodianRegions(ctx, params, optFns...)
		}
		return nil, errors.New("invalid key")
	})

	var (
		s, err = NewService(context.Background(), mockRatios, mockS3)
		hGet   = GetParametersHandler(s)
		params = new(ParametersV1)
	)
	if err != nil {
		t.Error("failed to make new service: ", err)
	}

	req, err := http.NewRequest("GET", "/parameters", nil)
	if err != nil {
		t.Error("failed to make new request: ", err)
	}

	rctx := chi.NewRouteContext()
	r := req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// add the parameters merge bucket to context
	r = req.WithContext(context.WithValue(r.Context(), appctx.ParametersMergeBucketCTXKey, "something"))

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

	if params.PayoutStatus == nil || params.PayoutStatus.Uphold != "processing" {
		t.Error("was expecting uphold to be set to processing")
	}
	if len(params.AutoContribute.Choices) == 0 {
		t.Error("was expecting more than one ac choices")
	}
}
