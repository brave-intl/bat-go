package ratios_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/services/ratios"
)

func newRouterTestContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.CoingeckoIDToSymbolCTXKey, map[string]string{
		"basic-attention-token": "bat",
		"ethereum":              "eth",
	})
	ctx = context.WithValue(ctx, appctx.CoingeckoSymbolToIDCTXKey, map[string]string{
		"bat": "basic-attention-token",
		"eth": "ethereum",
	})
	ctx = context.WithValue(ctx, appctx.CoingeckoContractToIDCTXKey, map[string]string{})
	ctx = context.WithValue(ctx, appctx.CoingeckoCoinLimitCTXKey, 2)
	ctx = context.WithValue(ctx, appctx.CoingeckoVsCurrencyLimitCTXKey, 2)
	ctx = context.WithValue(ctx, appctx.CoingeckoSupportedVsCurrenciesCTXKey, map[string]bool{
		"usd": true,
		"eur": true,
	})

	return ctx
}

func TestRouter(t *testing.T) {
	ctx := newRouterTestContext()
	s := ratios.NewService(ctx, nil, nil, nil)

	// Mount at the root to mirror the production wiring in cmd/rest_run.go.
	router := chi.NewRouter()
	router.Mount("/", ratios.Router(s))

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "history extracts coinID url param",
			path:       "/v2/history/coingecko/not-a-coin/usd/1d",
			wantStatus: http.StatusBadRequest,
			wantBody:   "not-a-coin",
		},
		{
			name:       "relative extracts vsCurrencies url param",
			path:       "/v2/relative/provider/coingecko/ethereum/not-a-currency/1d",
			wantStatus: http.StatusBadRequest,
			wantBody:   "not-a-currency",
		},
		{
			name:       "unknown route",
			path:       "/v2/unknown",
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
			}

			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("expected body to contain %q, got %s", tc.wantBody, rec.Body.String())
			}
		})
	}
}
