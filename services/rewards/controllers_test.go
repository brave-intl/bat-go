package rewards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/libs/clients/ratios"
	ratiosmock "github.com/brave-intl/bat-go/libs/clients/ratios/mock"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/go-chi/chi"
	"github.com/golang/mock/gomock"
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
				"usd": decimal.New(10, 0),
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
		}

		if *params.Key == "custodian-regions.json" {
			return mockS3CustodianRegions(ctx, params, optFns...)
		}

		return nil, errors.New("invalid key")
	})

	s := &Service{
		cfg:      Config{TOSVersion: 1},
		ratios:   mockRatios,
		s3Client: mockS3,
		cacheMu:  new(sync.RWMutex),
	}

	req, err := http.NewRequest(http.MethodGet, "/v1/parameters", nil)
	require.NoError(t, err)

	req = req.WithContext(context.WithValue(req.Context(), appctx.ParametersMergeBucketCTXKey, "something"))

	rw := httptest.NewRecorder()

	svr := &http.Server{Addr: ":8080", Handler: setupRouter(s)}
	svr.Handler.ServeHTTP(rw, req)

	require.Equal(t, http.StatusOK, rw.Code)

	params := &ParametersV1{}

	{
		err := json.Unmarshal(rw.Body.Bytes(), params)
		require.NoError(t, err)
	}

	require.NotNil(t, params.PayoutStatus)

	assert.Equal(t, "processing", params.PayoutStatus.Uphold)
	assert.ElementsMatch(t, []float64{3, 5, 7, 10, 20}, params.AutoContribute.Choices)
	assert.Equal(t, float64(10), params.BATRate)
	assert.Equal(t, 1, params.TOSVersion)
}

func setupRouter(s *Service) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/v1/parameters", GetParametersHandler(s).ServeHTTP)
	return r
}
