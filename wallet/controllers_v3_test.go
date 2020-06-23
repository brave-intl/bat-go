package wallet_test

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	mockledger "github.com/brave-intl/bat-go/utils/clients/ledger/mock"
	"github.com/brave-intl/bat-go/utils/logging"
	gomock "github.com/golang/mock/gomock"

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

func TestCreateWalletV3(t *testing.T) {
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
	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 200, got %d", resp.StatusCode))
	}
}

func TestCreateWalletV3_FailureSignature(t *testing.T) {
	var (
		db, _, _  = sqlmock.New()
		datastore = wallet.Datastore(
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
		ctx = context.Background()
		r   = httptest.NewRequest("POST", "/v3/wallet/brave", nil)
	)

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)

	for _, handler := range []handlers.AppHandler{
		wallet.CreateBraveWalletV3, wallet.CreateUpholdWalletV3,
	} {
		var w = httptest.NewRecorder()
		handlers.AppHandler(handler).ServeHTTP(w, r)
		if resp := w.Result(); resp.StatusCode != http.StatusForbidden {
			must(t, "invalid response", fmt.Errorf("expected 403, got %d", resp.StatusCode))
		}
	}
}
