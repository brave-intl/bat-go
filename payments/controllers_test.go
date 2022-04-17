package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
)

type mockSecretManager struct {
	err error
}

func (msm *mockSecretManager) RetrieveSecrets(ctx context.Context, uri string) (map[appctx.CTXKey]interface{}, error) {
	return map[appctx.CTXKey]interface{}{
		appctx.VersionCTXKey: "value", // version is a secret retrieved by retrieve secrets
	}, msm.err
}

func TestConfigurationHandler(t *testing.T) {
	s := &Service{
		baseCtx:   context.Background(),
		secretMgr: &mockSecretManager{},
	}

	secretsStored := false
	confStored := false

	r := chi.NewRouter()
	// startup our configuration middleware
	r.Use(s.ConfigurationMiddleware())

	r.Post("/conf", handlers.AppHandler(ConfigurationHandler(s)).ServeHTTP)

	r.Get("/valid", func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(appctx.VersionCTXKey).(string); ok && v == "value" {
			secretsStored = true
		}
		if v, ok := r.Context().Value(appctx.CommitCTXKey).(string); ok && v == "value" {
			confStored = true
		}
		w.Write([]byte("ok"))
	})

	reqBody := configurationHandlerRequest(map[appctx.CTXKey]interface{}{
		appctx.CommitCTXKey:     "value",       // commit is a configuration pushed in
		appctx.SecretsURICTXKey: "secrets uri", // tell configuration to pull new secrets
	})

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Error("err marshaling request body: ", err)
	}

	// conf request - setting config
	req := httptest.NewRequest("POST", "/conf", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// call another handler to check if we have the new values set
	req = httptest.NewRequest("GET", "/valid", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !secretsStored || !confStored {
		t.Error("should have stored secrets and conf for valid call")
	}
}
