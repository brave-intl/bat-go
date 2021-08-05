package wallet_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/middleware"
	mockgemini "github.com/brave-intl/bat-go/utils/clients/gemini/mock"
	mockreputation "github.com/brave-intl/bat-go/utils/clients/reputation/mock"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/wallet"
	"github.com/go-chi/chi"
	gomock "github.com/golang/mock/gomock"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	jose "gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
)

func must(t *testing.T, msg string, err error) {
	if err != nil {
		t.Errorf("%s: %s\n", msg, err)
	}
}

func signRequest(req *http.Request, publicKey httpsignature.Ed25519PubKey, privateKey ed25519.PrivateKey) error {
	var s httpsignature.SignatureParams
	s.Algorithm = httpsignature.ED25519
	s.KeyID = hex.EncodeToString(publicKey)
	s.Headers = []string{"digest", "(request-target)"}
	return s.Sign(privateKey, crypto.Hash(0), req)
}

type result struct{}

func (r result) LastInsertId() (int64, error) { return 1, nil }
func (r result) RowsAffected() (int64, error) { return 1, nil }

func TestLinkBraveWalletV3(t *testing.T) {
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
		// add the datastore to the context
		idFrom = uuid.NewV4()
		ctx    = middleware.AddKeyID(context.Background(), idFrom.String())
		idTo   = uuid.NewV4()
		r      = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v3/wallet/brave/%s/claim", idFrom),
			bytes.NewBufferString(fmt.Sprintf(`
				{
					"depositDestination": "%s"
				}`, idTo)),
		)
		mockReputation = mockreputation.NewMockClient(mockCtrl)
		handler        = wallet.LinkBraveDepositAccountV3(&wallet.Service{
			Datastore: datastore,
		})
		w = httptest.NewRecorder()
	)

	mockReputation.EXPECT().IsWalletOnPlatform(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		true,
		nil,
	)

	// begin linking tx
	mock.ExpectBegin()

	linkingID := uuid.NewV5(wallet.WalletClaimNamespace, idTo.String())
	// not before linked
	mock.ExpectQuery("^select linking_id from (.+)").WithArgs(idFrom, "brave").WillReturnError(sql.ErrNoRows)

	var max = sqlmock.NewRows([]string{"max"}).AddRow(4)
	var open = sqlmock.NewRows([]string{"used"}).AddRow(0)

	var custLinks = sqlmock.NewRows([]string{"custodian", "linking_id"}).AddRow("brave", linkingID.String())

	// linking limit checks
	mock.ExpectQuery("^select wc1.custodian, wc1.linking_id from wallet_custodian (.+)").WithArgs(linkingID).WillReturnRows(custLinks)
	mock.ExpectQuery("^select (.+)").WithArgs(linkingID, 4).WillReturnRows(max)
	mock.ExpectQuery("^select (.+)").WithArgs(linkingID).WillReturnRows(open)

	clRows := sqlmock.NewRows([]string{"created_at", "linked_at"}).
		AddRow(time.Now(), time.Now())

	// insert into wallet custodian
	mock.ExpectQuery("^insert into wallet_custodian (.+)").WithArgs(idFrom, "brave", uuid.NewV5(wallet.WalletClaimNamespace, idTo.String())).WillReturnRows(clRows)

	// updates the user_deposit_destination
	mock.ExpectExec("^update wallets (.+)").WithArgs(idTo, linkingID, "brave", idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// commit transaction
	mock.ExpectCommit()

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, mockReputation)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post("/v3/wallet/brave/{paymentID}/claim", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 201, got %d", resp.StatusCode))
	}
}

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
		// add the datastore to the context
		ctx     = context.Background()
		handler = wallet.CreateUpholdWalletV3
		r       = httptest.NewRequest("POST", "/v3/wallet/uphold", bytes.NewBufferString(`{
				"signedCreationRequest": "eyJib2R5Ijp7ImRlbm9taW5hdGlvbiI6eyJhbW91bnQiOiIwIiwiY3VycmVuY3kiOiJCQVQifSwiZGVzdGluYXRpb24iOiJhNmRmZjJiYS1kMGQxLTQxYzQtOGU1Ni1hMjYwNWJjYWY0YWYifSwiaGVhZGVycyI6eyJkaWdlc3QiOiJTSEEtMjU2PWR2RTAzVHdpRmFSR0c0MUxLSkR4aUk2a3c5M0h0cTNsclB3VllldE5VY1E9Iiwic2lnbmF0dXJlIjoia2V5SWQ9XCJwcmltYXJ5XCIsYWxnb3JpdGhtPVwiZWQyNTUxOVwiLGhlYWRlcnM9XCJkaWdlc3RcIixzaWduYXR1cmU9XCJkcXBQdERESXE0djNiS1V5eHB6Q3Vyd01nSzRmTWk1MUJjakRLc2pTak90K1h1MElZZlBTMWxEZ01aRkhiaWJqcGh0MVd3V3l5enFad3lVNW0yN1FDUT09XCIifSwib2N0ZXRzIjoie1wiZGVub21pbmF0aW9uXCI6e1wiYW1vdW50XCI6XCIwXCIsXCJjdXJyZW5jeVwiOlwiQkFUXCJ9LFwiZGVzdGluYXRpb25cIjpcImE2ZGZmMmJhLWQwZDEtNDFjNC04ZTU2LWEyNjA1YmNhZjRhZlwifSJ9"}`))
	)
	// no logger, setup
	ctx, _ = logging.SetupLogger(ctx)

	// setup sqlmock
	mock.ExpectExec("^INSERT INTO wallets (.+)").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(result{})

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)

	r = r.WithContext(ctx)

	var w = httptest.NewRecorder()
	handlers.AppHandler(handler).ServeHTTP(w, r)
	if resp := w.Result(); resp.StatusCode != http.StatusBadRequest {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected 400, got %d", resp.StatusCode))
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
		id      = uuid.NewV4()
		r       = httptest.NewRequest("GET", fmt.Sprintf("/v3/wallet/%s", id), nil)
		handler = wallet.GetWalletV3
		w       = httptest.NewRecorder()
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

func TestLinkBitFlyerWalletV3(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	// setup jwt token for the test
	var secret = []byte("a jwt secret")
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: secret}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		panic(err)
	}

	var (
		idFrom      = uuid.NewV4()
		idTo        = uuid.NewV4()
		accountHash = uuid.NewV4()
		timestamp   = time.Now()
	)

	h := sha256.New()
	if _, err := h.Write([]byte(idFrom.String())); err != nil {
		panic(err)
	}

	externalAccountID := hex.EncodeToString(h.Sum(nil))

	cl := wallet.BitFlyerLinkingInfo{
		DepositID:         idTo.String(),
		RequestID:         "1",
		AccountHash:       accountHash.String(),
		ExternalAccountID: externalAccountID,
		Timestamp:         timestamp,
	}

	tokenString, err := jwt.Signed(sig).Claims(cl).CompactSerialize()
	if err != nil {
		panic(err)
	}

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
		ctx = middleware.AddKeyID(context.WithValue(context.Background(), appctx.BitFlyerJWTKeyCTXKey, []byte(secret)), idFrom.String())
		r   = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v3/wallet/bitflyer/%s/claim", idFrom),
			bytes.NewBufferString(fmt.Sprintf(`
				{
					"linkingInfo": "%s"
				}`, tokenString)),
		)
		mockReputation = mockreputation.NewMockClient(mockCtrl)
		handler        = wallet.LinkBitFlyerDepositAccountV3(&wallet.Service{
			Datastore: datastore,
		})
		w = httptest.NewRecorder()
	)
	mock.ExpectExec("^insert (.+)").WithArgs("1").WillReturnResult(sqlmock.NewResult(1, 1))

	// begin linking tx
	mock.ExpectBegin()

	// make sure old linking id matches new one for same custodian
	linkingID := uuid.NewV5(wallet.WalletClaimNamespace, accountHash.String())

	// this wallet has been linked prior, with the same linking id that the request is with
	// SHOULD SKIP THE linking limit checks
	var linkingIDRows = sqlmock.NewRows([]string{"linking_id"}).AddRow(linkingID)
	mock.ExpectQuery("^select linking_id from (.+)").WithArgs(idFrom, "bitflyer").WillReturnRows(linkingIDRows)

	clRows := sqlmock.NewRows([]string{"created_at", "linked_at"}).
		AddRow(time.Now(), time.Now())

	// insert into wallet custodian
	mock.ExpectQuery("^insert into wallet_custodian (.+)").WithArgs(idFrom, "bitflyer", uuid.NewV5(wallet.WalletClaimNamespace, accountHash.String())).WillReturnRows(clRows)

	// updates the link to the wallet_custodian record in wallets
	mock.ExpectExec("^update wallets (.+)").WithArgs(idTo, linkingID, "bitflyer", idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// commit transaction
	mock.ExpectCommit()

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, mockReputation)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post("/v3/wallet/bitflyer/{paymentID}/claim", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode))
	}
}

func TestLinkGeminiWalletV3FirstLinking(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	var (
		// setup test variables
		idFrom    = uuid.NewV4()
		ctx       = middleware.AddKeyID(context.Background(), idFrom.String())
		accountID = uuid.NewV4()
		idTo      = accountID

		// setup db mocks
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
		linkingInfo = "this is the fake jwt for linking_info"

		// setup mock clients
		mockReputationClient = mockreputation.NewMockClient(mockCtrl)
		mockGeminiClient     = mockgemini.NewMockClient(mockCtrl)

		// this is our main request
		r = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v3/wallet/gemini/%s/claim", idFrom),
			bytes.NewBufferString(fmt.Sprintf(`
				{
					"linking_info": "%s",
					"recipient_id": "%s"
				}`, linkingInfo, idTo)),
		)
		handler = wallet.LinkGeminiDepositAccountV3(&wallet.Service{
			Datastore: datastore,
		})
		w = httptest.NewRecorder()
	)

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, mockReputationClient)
	ctx = context.WithValue(ctx, appctx.GeminiClientCTXKey, mockGeminiClient)

	mockGeminiClient.EXPECT().ValidateAccount(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		accountID.String(),
		nil,
	)

	// begin linking tx
	mock.ExpectBegin()

	// make sure old linking id matches new one for same custodian
	linkingID := uuid.NewV5(wallet.WalletClaimNamespace, idTo.String())
	// not before linked
	mock.ExpectQuery("^select linking_id from (.+)").WithArgs(idFrom, "gemini").WillReturnError(sql.ErrNoRows)

	var max = sqlmock.NewRows([]string{"max"}).AddRow(4)
	var open = sqlmock.NewRows([]string{"used"}).AddRow(0)

	var custLinks = sqlmock.NewRows([]string{"custodian", "linking_id"}).AddRow("gemini", linkingID.String())

	// linking limit checks
	mock.ExpectQuery("^select wc1.custodian, wc1.linking_id from wallet_custodian (.+)").WithArgs(linkingID).WillReturnRows(custLinks)
	mock.ExpectQuery("^select (.+)").WithArgs(linkingID, 4).WillReturnRows(max)
	mock.ExpectQuery("^select (.+)").WithArgs(linkingID).WillReturnRows(open)

	clRows := sqlmock.NewRows([]string{"created_at", "linked_at"}).
		AddRow(time.Now(), time.Now())

	// insert into wallet custodian
	mock.ExpectQuery("^insert into wallet_custodian (.+)").WithArgs(idFrom, "gemini", uuid.NewV5(wallet.WalletClaimNamespace, accountID.String())).WillReturnRows(clRows)

	// updates the link to the wallet_custodian record in wallets
	mock.ExpectExec("^update wallets (.+)").WithArgs(idTo, linkingID, "gemini", idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// commit transaction
	mock.ExpectCommit()

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post("/v3/wallet/gemini/{paymentID}/claim", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode))
	}
}

func TestLinkGeminiWalletV3(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	var (
		// setup test variables
		idFrom    = uuid.NewV4()
		ctx       = middleware.AddKeyID(context.Background(), idFrom.String())
		accountID = uuid.NewV4()
		idTo      = accountID

		// setup db mocks
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
		linkingInfo = "this is the fake jwt for linking_info"

		// setup mock clients
		mockReputationClient = mockreputation.NewMockClient(mockCtrl)
		mockGeminiClient     = mockgemini.NewMockClient(mockCtrl)

		// this is our main request
		r = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v3/wallet/gemini/%s/claim", idFrom),
			bytes.NewBufferString(fmt.Sprintf(`
				{
					"linking_info": "%s",
					"recipient_id": "%s"
				}`, linkingInfo, idTo)),
		)
		handler = wallet.LinkGeminiDepositAccountV3(&wallet.Service{
			Datastore: datastore,
		})
		w = httptest.NewRecorder()
	)

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)
	ctx = context.WithValue(ctx, appctx.ReputationClientCTXKey, mockReputationClient)
	ctx = context.WithValue(ctx, appctx.GeminiClientCTXKey, mockGeminiClient)

	mockGeminiClient.EXPECT().ValidateAccount(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		accountID.String(),
		nil,
	)

	// begin linking tx
	mock.ExpectBegin()

	// make sure old linking id matches new one for same custodian
	linkingID := uuid.NewV5(wallet.WalletClaimNamespace, idTo.String())
	var linkingIDRows = sqlmock.NewRows([]string{"linking_id"}).AddRow(linkingID)
	mock.ExpectQuery("^select linking_id from (.+)").WithArgs(idFrom, "gemini").WillReturnRows(linkingIDRows)

	// this wallet has been linked prior, with the same linking id that the request is with
	// SHOULD SKIP THE linking limit checks
	clRows := sqlmock.NewRows([]string{"created_at", "linked_at"}).
		AddRow(time.Now(), time.Now())

	// insert into wallet custodian
	mock.ExpectQuery("^insert into wallet_custodian (.+)").WithArgs(idFrom, "gemini", uuid.NewV5(wallet.WalletClaimNamespace, accountID.String())).WillReturnRows(clRows)

	// updates the link to the wallet_custodian record in wallets
	mock.ExpectExec("^update wallets (.+)").WithArgs(idTo, linkingID, "gemini", idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// commit transaction
	mock.ExpectCommit()

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post("/v3/wallet/gemini/{paymentID}/claim", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		t.Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		t.Logf("%s, %+v\n", body, err)
		must(t, "invalid response", fmt.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode))
	}
}

func TestDisconnectCustodianLinkV3(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	var (
		// setup test variables
		idFrom = uuid.NewV4()
		ctx    = middleware.AddKeyID(context.Background(), idFrom.String())

		// setup db mocks
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

		// this is our main request
		r = httptest.NewRequest(
			"DELETE",
			fmt.Sprintf("/v3/wallet/gemini/%s/claim", idFrom), nil)

		handler = wallet.DisconnectCustodianLinkV3(&wallet.Service{
			Datastore: datastore,
		})
		w = httptest.NewRecorder()
	)

	// create transaction
	mock.ExpectBegin()

	// removes the link to the user_deposit_destination record in wallets
	mock.ExpectExec("^update wallets (.+)").WithArgs(idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// updates the disconnected date on the record, and returns no error and one changed row
	mock.ExpectExec("^update wallet_custodian(.+)").WithArgs(idFrom).WillReturnResult(sqlmock.NewResult(1, 1))

	// commit transaction because we are done disconnecting
	mock.ExpectCommit()

	ctx = context.WithValue(ctx, appctx.DatastoreCTXKey, datastore)
	ctx = context.WithValue(ctx, appctx.RODatastoreCTXKey, roDatastore)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Delete("/v3/wallet/{custodian}/{paymentID}/claim", handlers.AppHandler(handler).ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusOK {
		must(t, "invalid response", fmt.Errorf("expected %d, got %d", http.StatusOK, resp.StatusCode))
	}
}
