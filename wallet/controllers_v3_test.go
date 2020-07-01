package wallet_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"testing"

	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/jmoiron/sqlx"
)

func must(t *testing.T, msg string, err error) {
	if err != nil {
		t.Errorf("%s: %s\n", msg, err)
	}
}

func signRequest(req *http.Request, publicKey httpsignature.Ed25519PubKey, privateKey ed25519.PrivateKey) error {
	var s httpsignature.Signature
	s.Algorithm = httpsignature.ED25519
	s.KeyID = hex.EncodeToString(publicKey)
	s.Headers = []string{"digest", "(request-target)"}
	return s.Sign(privateKey, crypto.Hash(0), req)
}

type result struct{}

func (r result) LastInsertId() (int64, error) { return 1, nil }
func (r result) RowsAffected() (int64, error) { return 1, nil }

func TestCreateBraveWalletV3(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	var (
		db, mock, _ = sqlmock.New()
		datastore   = wallet.Datastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		roDatastore = wallet.ReadOnlyDatastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		mockLedger = mockledger.NewMockClient(mockCtrl)
		// add the datastore to the context
		ctx     = context.Background()
		handler = wallet.CreateBraveWalletV3
		r       = httptest.NewRequest("POST", "/v3/wallet/brave", nil)
	)
	// no logger, setup
	ctx, _ = logging.SetupLogger(ctx)

	// setup sqlmock
	mock.ExpectExec("^INSERT INTO wallets (.+)").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(result{})

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.LedgerServiceCTXKey, mockLedger)

	// setup keypair
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	must(t, "failed to generate keypair", err)

	err = signRequest(r, publicKey, privKey)
	must(t, "failed to sign request", err)

	r = r.WithContext(ctx)

	var w = httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(w, r)
	if resp := w.Result(); resp.StatusCode != http.StatusCreated {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 200, got %d", resp.StatusCode))
	}
}

func TestCreateUpholdWalletV3(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	var (
		db, mock, _ = sqlmock.New()
		datastore   = wallet.Datastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		roDatastore = wallet.ReadOnlyDatastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		mockLedger = mockledger.NewMockClient(mockCtrl)
		// add the datastore to the context
		ctx     = context.Background()
		handler = wallet.CreateUpholdWalletV3
		r       = httptest.NewRequest("POST", "/v3/wallet/uphold", bytes.NewBufferString(`{
				"signedCreationRequest": "123",
				"anonymousAccount": "650e1323-c2c2-444c-8eeb-c920b230c95c"
			}`))
	)
	// no logger, setup
	ctx, _ = logging.SetupLogger(ctx)

	// setup sqlmock
	mock.ExpectExec("^INSERT INTO wallets (.+)").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(result{})

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.LedgerServiceCTXKey, mockLedger)

	// setup keypair
	publicKey, privKey, err := httpsignature.GenerateEd25519Key(nil)
	must(t, "failed to generate keypair", err)
	err = signRequest(r, publicKey, privKey)
	must(t, "failed to sign request", err)

	r = r.WithContext(ctx)

	b, _ := httputil.DumpRequest(r, true)
	fmt.Printf("\n\n%s\n\n", b)

	var w = httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(w, r)
	if resp := w.Result(); resp.StatusCode != http.StatusServiceUnavailable {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 503, got %d", resp.StatusCode))
	}
}

func TestGetWalletV3(t *testing.T) {
	var (
		db, mock, _ = sqlmock.New()
		datastore   = wallet.Datastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		roDatastore = wallet.ReadOnlyDatastore(
			&wallet.Postgres{
				grantserver.Postgres{
					DB: sqlx.NewDb(db, "postgres"),
				},
			})
		// add the datastore to the context
		ctx     = context.Background()
		r       = httptest.NewRequest("GET", "/v3/wallet/7def9cda-6a14-4fa1-be86-43da80e56d2c", nil)
		handler = wallet.GetWalletV3
		w       = httptest.NewRecorder()
		id, _   = uuid.FromString("7def9cda-6a14-4fa1-be86-43da80e56d2c")
		rows    = sqlmock.NewRows([]string{"id", "provider", "provider_id", "public_key", "provider_linking_id", "anonymous_address"}).
			AddRow(id, "brave", "", "12345", id, id)
	)

	mock.ExpectQuery("^select (.+)").WithArgs(id).WillReturnRows(rows)

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Get("/v3/wallet/{paymentID}", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 201, got %d", resp.StatusCode))
	}
}
