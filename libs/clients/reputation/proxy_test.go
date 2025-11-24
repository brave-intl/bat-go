//go:build integration

package reputation

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestProxyRouter(t *testing.T) {
	reputationServer := os.Getenv("REPUTATION_SERVER")
	reputationToken := os.Getenv("REPUTATION_TOKEN")
	if len(reputationServer) == 0 || len(reputationToken) == 0 {
		t.Skip("skipping test; REPUTATION_SERVER or REPUTATION_TOKEN not set")
	}

	handler := ProxyRouter(reputationServer, reputationToken)
	req, err := http.NewRequest("POST", "/v1/devicecheck/attestations", nil)
	if err != nil {
		t.Fatal("Error creating request")
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Response should be 400, not 403 (which would indicate authorization wasn't added correctly)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("Response code did not match: %d != %d", http.StatusBadRequest, rr.Code)
	}
}
