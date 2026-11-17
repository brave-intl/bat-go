package promotion_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/services/promotion"
)

func TestRouter(t *testing.T) {
	// A nil service is safe for these cases because the handlers reject invalid
	// requests before touching the service.
	router := promotion.Router(nil, time.Now().Add(time.Hour))

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "claim summary route matches claimType url param",
			path:       "/ads/grants/summary",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "claim summary route validates paymentID query param",
			path:       "/ads/grants/summary?paymentId=not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown route",
			path:       "/unknown/unknown/unknown/unknown",
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}
