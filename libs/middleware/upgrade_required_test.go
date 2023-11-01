package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpgradeRequiredByMiddleware(t *testing.T) {
	// after cutoff
	wrappedHandler := NewUpgradeRequiredByMiddleware(time.Now().Add(-1 * time.Second))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()
	resp, _ := http.Get(server.URL)
	assert.Equal(t, resp.StatusCode, http.StatusUpgradeRequired, "status code should be upgrade required")

	// not yet cutoff
	wrappedHandler = NewUpgradeRequiredByMiddleware(time.Now().Add(1 * time.Second))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server = httptest.NewServer(wrappedHandler)
	defer server.Close()
	resp, _ = http.Get(server.URL)
	assert.Equal(t, resp.StatusCode, http.StatusOK, "status code should be OK")
}
