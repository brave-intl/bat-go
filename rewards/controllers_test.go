package rewards

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi"
)

func TestGetParametersController(t *testing.T) {
	var (
		h      = GetParameters(NewService(context.Background()))
		params = new(Parameters)
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
		t.Error("was expecting an ok response: ", rr.Code)
	}

	if err = json.Unmarshal(rr.Body.Bytes(), &params); err != nil {
		t.Error("should be no error with unmarshalling response: ", err)
	}

	// TODO: make some assertions
}
