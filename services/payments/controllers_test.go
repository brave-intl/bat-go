package payments

//
//import (
//	"bytes"
//	"context"
//	"crypto/rand"
//	"encoding/base64"
//	"encoding/hex"
//	"encoding/json"
//	"fmt"
//	"io"
//	"net/http"
//	"net/http/httptest"
//	"testing"
//
//	appctx "github.com/brave-intl/bat-go/libs/context"
//	"github.com/brave-intl/bat-go/libs/cryptography"
//	"github.com/brave-intl/bat-go/libs/handlers"
//	"github.com/go-chi/chi"
//	"golang.org/x/crypto/nacl/box"
//)
//
//type mockSecretManager struct {
//	err    error
//	result []byte
//}
//
//func (msm *mockSecretManager) RetrieveSecrets(ctx context.Context, uri string) ([]byte, error) {
//	return msm.result, msm.err
//}
//
//func TestPatchConfigurationHandler(t *testing.T) {
//	_, s, err := NewService(context.Background())
//	if err != nil {
//		t.Error("failed to init service: ", err)
//	}
//
//	secretsStored := false
//	confStored := false
//
//	r := chi.NewRouter()
//	// startup our configuration middleware
//	r.Use(s.ConfigurationMiddleware)
//
//	// get the public key
//	r.Get("/conf", handlers.AppHandler(GetConfigurationHandler(s)).ServeHTTP)
//
//	// conf request in order to get service's pubkey, so we can encrypt a secret
//	req1 := httptest.NewRequest("GET", "/conf", nil)
//	w1 := httptest.NewRecorder()
//	r.ServeHTTP(w1, req1)
//
//	var getConf = getConfResponse{}
//	if err := json.NewDecoder(w1.Result().Body).Decode(&getConf); err != nil {
//		t.Error("failed to decode config response", err)
//	}
//
//	// servicePubKey is the ed25519 key we will use for encrypting the config encryption key
//	servicePubKey, err := hex.DecodeString(getConf.PublicKey)
//	if err != nil {
//		t.Error("failed to decode config response", err)
//	}
//
//	// generate an ephemeral sender keypair
//	senderPubKey, senderPrivKey, err := box.GenerateKey(rand.Reader)
//	if err != nil {
//		t.Error("failed to create sender keypair", err)
//	}
//
//	var secretKey [32]byte
//	if _, err := io.ReadFull(rand.Reader, secretKey[:]); err != nil {
//		t.Error("failed to create random secret key", err)
//	}
//
//	// encrypt a secret
//	c, n, err := cryptography.EncryptMessage(secretKey, []byte(`{"version":"value"}`))
//	if err != nil {
//		t.Error("failed to encrypt our secrets", err)
//	}
//
//	// put the nonce at the start of our message
//	c = append(n[:], c...)
//
//	// now for the encryption of the secret for transport
//	var nonce [24]byte
//	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
//		t.Error("failed to generate our nonce", err)
//	}
//
//	var rpk [32]byte
//	copy(rpk[:], servicePubKey[:32])
//
//	var spk [32]byte
//	copy(spk[:], senderPrivKey[:32])
//
//	// encrypt the key which was used to encrypt secret
//	encrypted := box.Seal(nonce[:], secretKey[:], &nonce, &rpk, &spk)
//
//	// now put this in our configuration
//	s.secretMgr = &mockSecretManager{
//		result: c, // c is our encrypted config items
//		err:    nil,
//	}
//
//	// set the values
//	// @TODO: Define PatchConfigurationHandler
//	// r.Patch("/conf", handlers.AppHandler(PatchConfigurationHandler(s)).ServeHTTP)
//
//	r.Get("/valid", func(w http.ResponseWriter, r *http.Request) {
//		if v, ok := r.Context().Value(appctx.VersionCTXKey).(string); ok && v == "value" {
//			secretsStored = true
//		}
//		if v, ok := r.Context().Value(appctx.CommitCTXKey).(string); ok && v == "value" {
//			confStored = true
//		}
//		fmt.Println(w.Write([]byte("ok")))
//	})
//
//	reqBody := configurationHandlerRequest(map[appctx.CTXKey]interface{}{
//		appctx.CommitCTXKey:                  "value",                                      // commit is a configuration pushed in
//		appctx.SecretsURICTXKey:              "secrets uri",                                // tell configuration to pull new secrets
//		appctx.PaymentsEncryptionKeyCTXKey:   base64.StdEncoding.EncodeToString(encrypted), // tell configuration to pull new secrets
//		appctx.PaymentsSenderPublicKeyCTXKey: hex.EncodeToString(senderPubKey[:]),          // tell configuration to pull new secrets
//		appctx.PaymentsKMSWrapperCTXKey:      "key",                                        // tell configuration which kms key to decrypt secret object with
//	})
//
//	body, err := json.Marshal(reqBody)
//	if err != nil {
//		t.Error("err marshaling request body: ", err)
//	}
//
//	// conf request - setting config
//	req := httptest.NewRequest("PATCH", "/conf", bytes.NewBuffer(body))
//	w := httptest.NewRecorder()
//	r.ServeHTTP(w, req)
//
//	// call another handler to check if we have the new values set
//	req = httptest.NewRequest("GET", "/valid", nil)
//	w = httptest.NewRecorder()
//	r.ServeHTTP(w, req)
//
//	if !secretsStored || !confStored {
//		t.Error("should have stored secrets and conf for valid call")
//	}
//}
//
//func TestGetConfigurationHandler(t *testing.T) {
//	_, s, err := NewService(context.Background())
//	if err != nil {
//		t.Error("failed to init payment service: ", err)
//	}
//
//	r := chi.NewRouter()
//
//	r.Get("/conf", handlers.AppHandler(GetConfigurationHandler(s)).ServeHTTP)
//
//	// conf request - getting config
//	req := httptest.NewRequest("GET", "/conf", nil)
//	w := httptest.NewRecorder()
//	r.ServeHTTP(w, req)
//
//	// get public key from response writer body
//	resp := w.Result()
//	conf := getConfResponse{}
//
//	if err := json.NewDecoder(resp.Body).Decode(&conf); err != nil {
//		t.Error("failed to decode response body get conf: ", err)
//	}
//
//	returnedPubKey, err := hex.DecodeString(conf.PublicKey)
//	if err != nil {
//		t.Error("failed to decode response body pubkey: ", err)
//	}
//
//	if bytes.Compare(s.pubKey[:], returnedPubKey) != 0 {
//		t.Error("public key does not match")
//	}
//}
//
