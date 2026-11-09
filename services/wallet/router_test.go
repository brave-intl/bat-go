package wallet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouterCORSPreflight(t *testing.T) {
	type testCase struct {
		name         string
		dappOrigins  []string
		requestHost  string
		expectACAO   bool
		allowedValue string
		wildcard     bool
	}

	tests := []testCase{
		{
			name:         "origin on the allow list is echoed",
			dappOrigins:  []string{"https://allowed.example.com"},
			requestHost:  "https://allowed.example.com",
			expectACAO:   true,
			allowedValue: "https://allowed.example.com",
		},
		{
			name:        "origin not on the allow list is not echoed",
			dappOrigins: []string{"https://allowed.example.com"},
			requestHost: "https://denied.example.com",
			expectACAO:  false,
		},
		{
			name:        "empty allow list falls back to wildcard",
			dappOrigins: []string{},
			requestHost: "https://allowed.example.com",
			expectACAO:  true,
			wildcard:    true,
		},
		{
			name:        "blank origin entry allows no origin",
			dappOrigins: []string{""},
			requestHost: "https://allowed.example.com",
			expectACAO:  false,
		},
		{
			name:         "second entry of the allow list is honored",
			dappOrigins:  []string{"https://first.example.com", "https://second.example.com"},
			requestHost:  "https://second.example.com",
			expectACAO:   true,
			allowedValue: "https://second.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := Router(context.TODO(), &Service{}, RouterConfig{
				DAppOrigins:       tc.dappOrigins,
				InstrumentHandler: noOpInstrumentHandler(),
			})

			req := httptest.NewRequest(http.MethodOptions, "/v3/wallet/challenges", nil)
			req.Header.Set("Origin", tc.requestHost)
			req.Header.Set("Access-Control-Request-Method", http.MethodPost)

			rw := httptest.NewRecorder()
			router.ServeHTTP(rw, req)

			acao := rw.Header().Get("Access-Control-Allow-Origin")
			if tc.wildcard {
				assert.Equal(t, "*", acao)
			} else if tc.expectACAO {
				assert.Equal(t, tc.allowedValue, acao)
			} else {
				assert.Empty(t, acao)
			}
		})
	}
}

func TestRouterLinkDrainFlag(t *testing.T) {
	paymentID := uuid.NewV4().String()

	t.Run("enabled registers the drain routes", func(t *testing.T) {
		router := Router(context.TODO(), &Service{}, RouterConfig{
			LinkDrainEnabled:  true,
			DAppOrigins:       []string{"https://allowed.example.com"},
			InstrumentHandler: noOpInstrumentHandler(),
		})

		req := httptest.NewRequest(http.MethodPost, "/v3/wallet/solana/"+paymentID+"/connect", nil)
		rw := httptest.NewRecorder()
		router.ServeHTTP(rw, req)

		// No Origin header: the registered POST handler rejects with 403
		// (controllers_v3.go origin check), distinct from 404/405.
		require.Equal(t, http.StatusForbidden, rw.Code, rw.Body.String())
	})

	t.Run("disabled does not register the drain routes", func(t *testing.T) {
		router := Router(context.TODO(), &Service{}, RouterConfig{
			LinkDrainEnabled:  false,
			DAppOrigins:       []string{"https://allowed.example.com"},
			InstrumentHandler: noOpInstrumentHandler(),
		})

		req := httptest.NewRequest(http.MethodPost, "/v3/wallet/solana/"+paymentID+"/connect", nil)
		rw := httptest.NewRecorder()
		router.ServeHTTP(rw, req)

		assert.Equal(t, http.StatusNotFound, rw.Code, rw.Body.String())
	})
}

func TestRouterZeroConfig(t *testing.T) {
	// The zero value must be safe: no panic on registration and no instrumentation.
	router := Router(context.TODO(), &Service{}, RouterConfig{})
	require.NotNil(t, router)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rw := httptest.NewRecorder()
	router.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)
}
