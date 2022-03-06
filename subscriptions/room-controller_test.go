package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/go-chi/chi"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/suite"
)

type RoomTestSuite struct {
	service *Service
	suite.Suite
}

func must(t *testing.T, msg string, err error) {
	if err != nil {
		t.Errorf("%s: %s\n", msg, err)
	}
}

func ValidateIsGroupRoom(ctx context.Context, res *httptest.ResponseRecorder, expected bool) error {
	var dat map[string]interface{}
	if err := json.Unmarshal(res.Body.Bytes(), &dat); err != nil {
		return fmt.Errorf("failed to parse json response %v", err)
	}

	var jwt RoomClaims
	if err := MustVerify(ctx, dat["jwt"].(string), &jwt); err != nil {
		return fmt.Errorf("failed to parse jwt %v", err)
	}

	braveFeatures := jwt.Context["x-brave-features"]
	if braveFeatures == nil {
		return fmt.Errorf("context.x-brave-features not found in jwt: %+v", jwt)
	}

	groupRoom := braveFeatures.(map[string]interface{})["group-room"]

	if groupRoom == nil {
		return fmt.Errorf("context.x-brave-features.group-room not found in jwt: %+v", jwt)
	}

	actual, err := strconv.ParseBool(groupRoom.(string))
	if err != nil {
		return fmt.Errorf("failed to parse context.x-brave-features.group-room %v", err)
	}

	if actual != expected {
		return fmt.Errorf("expected context.x-brave-features.group-room to be %v was %v", expected, groupRoom)
	}

	return nil
}

func TestRoomSuite(t *testing.T) {
	suite.Run(t, new(RoomTestSuite))
}

func (suite *RoomTestSuite) SetupSuite() {
	pg, err := NewPostgres("", true, "subscriptions")
	suite.Require().NoError(err, "Failed to get postgres conn")

	suite.service = &Service{
		Datastore: pg,
	}
}

func (suite *RoomTestSuite) TearDownTest() {
	suite.CleanDB()
}

func (suite *RoomTestSuite) CleanDB() {
	tables := []string{"rooms"}

	pg, err := NewPostgres("", false, "subscriptions")
	suite.Require().NoError(err, "Failed to get postgres conn")

	for _, table := range tables {
		_, err = pg.RawDB().Exec("delete from " + table)
		suite.Require().NoError(err, "Failed to get clean table")
	}
}

func (suite *RoomTestSuite) TestShouldCreateFreeRoomIfNoCredentialAndFromPremiumBeta() {
	var (
		roomName    = "5wOWXZkVb2jbX0ec5FeeuYytBj6LwRRc8S2DAYaJ9Pc"
		db, mock, _ = sqlmock.New()
		idFrom      = uuid.NewV4()
		ctx         = middleware.AddKeyID(context.Background(), idFrom.String())
		r           = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v1/rooms/%s", roomName),
			bytes.NewBufferString(`{"premium_beta": true}`),
		)
		handler       = CreateRoomV1Router(suite.service)
		privateKey, _ = getPrivKey("", "/src/assets/jose_sample.key")
		w             = httptest.NewRecorder()
	)
	suite.service.Datastore = Datastore(
		&Postgres{
			grantserver.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
		})

	rooms := sqlmock.NewRows([]string{"name", "tier", "head_count", "created_at", "terminated_at"}).
		AddRow(roomName, "free", 1, time.Now(), nil)
	mock.ExpectQuery("INSERT INTO rooms (.+)").WithArgs(roomName, "free").WillReturnRows(rooms)

	ctx = context.WithValue(ctx, appctx.JAASTenantIDCTXKey, "JTID")
	ctx = context.WithValue(ctx, appctx.JAASKeyIDCTXKey, "JKID")
	ctx = context.WithValue(ctx, appctx.JAASPrivateKeyCTXKey, privateKey)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post(r.RequestURI, handler.ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusCreated {
		suite.T().Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		suite.T().Logf("%s, %+v\n", body, err)
		must(suite.T(), "invalid response", fmt.Errorf("expected %d, got %d", http.StatusCreated, resp.StatusCode))
	}

	if err := ValidateIsGroupRoom(ctx, w, false); err != nil {
		suite.T().Error(err)
	}
}

func (suite *RoomTestSuite) TestCreatePaidRoomIfCredentialSupplied() {
	var (
		roomName    = "5wOWXZkVb2jbX0ec5FeeuYytBj6LwRRc8S2DAYaJ9Pc"
		db, mock, _ = sqlmock.New()
		idFrom      = uuid.NewV4()
		ctx         = middleware.AddKeyID(context.Background(), idFrom.String())
		r           = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v1/rooms/%s", roomName),
			bytes.NewBufferString(`{"mauP": false}`),
		)
		handler       = CreateRoomV1Router(suite.service)
		privateKey, _ = getPrivKey("", "/src/assets/jose_sample.key")
		w             = httptest.NewRecorder()
	)
	suite.service.Datastore = Datastore(
		&Postgres{
			grantserver.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
		})

	rooms := sqlmock.NewRows([]string{"name", "tier", "head_count", "created_at", "terminated_at"}).
		AddRow(roomName, "paid", 1, time.Now(), nil)
	mock.ExpectQuery("INSERT INTO rooms (.+)").WithArgs(roomName, "paid").WillReturnRows(rooms)

	ctx = context.WithValue(ctx, appctx.JAASTenantIDCTXKey, "JTID")
	ctx = context.WithValue(ctx, appctx.JAASKeyIDCTXKey, "JKID")
	ctx = context.WithValue(ctx, appctx.JAASPrivateKeyCTXKey, privateKey)
	ctx = context.WithValue(ctx, SkuAuthContextKey, "brave-talk-premium")

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post(r.RequestURI, handler.ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusCreated {
		suite.T().Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		suite.T().Logf("%s, %+v\n", body, err)
		must(suite.T(), "invalid response", fmt.Errorf("expected %d, got %d", http.StatusCreated, resp.StatusCode))
	}

	if err := ValidateIsGroupRoom(ctx, w, true); err != nil {
		suite.T().Error(err)
	}
}

func (suite *RoomTestSuite) TestShouldCreateFreeRoomIfNoCredentialAndNotFromPremiumBeta() {
	var (
		roomName    = "5wOWXZkVb2jbX0ec5FeeuYytBj6LwRRc8S2DAYaJ9Pc"
		db, mock, _ = sqlmock.New()
		idFrom      = uuid.NewV4()
		ctx         = middleware.AddKeyID(context.Background(), idFrom.String())
		r           = httptest.NewRequest(
			"POST",
			fmt.Sprintf("/v1/rooms/%s", roomName),
			bytes.NewBufferString(`{"mauP": false}`),
		)
		handler       = CreateRoomV1Router(suite.service)
		privateKey, _ = getPrivKey("", "/src/assets/jose_sample.key")
		w             = httptest.NewRecorder()
	)
	suite.service.Datastore = Datastore(
		&Postgres{
			grantserver.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
		})

	rooms := sqlmock.NewRows([]string{"name", "tier", "head_count", "created_at", "terminated_at"}).
		AddRow(roomName, "free", 1, time.Now(), nil)
	mock.ExpectQuery("INSERT INTO rooms (.+)").WithArgs(roomName, "free").WillReturnRows(rooms)

	ctx = context.WithValue(ctx, appctx.JAASTenantIDCTXKey, "JTID")
	ctx = context.WithValue(ctx, appctx.JAASKeyIDCTXKey, "JKID")
	ctx = context.WithValue(ctx, appctx.JAASPrivateKeyCTXKey, privateKey)

	r = r.WithContext(ctx)

	router := chi.NewRouter()
	router.Post(r.RequestURI, handler.ServeHTTP)
	router.ServeHTTP(w, r)

	if resp := w.Result(); resp.StatusCode != http.StatusCreated {
		suite.T().Logf("%+v\n", resp)
		body, err := ioutil.ReadAll(resp.Body)
		suite.T().Logf("%s, %+v\n", body, err)
		must(suite.T(), "invalid response", fmt.Errorf("expected %d, got %d", http.StatusCreated, resp.StatusCode))
	}

	if err := ValidateIsGroupRoom(ctx, w, false); err != nil {
		suite.T().Error(err)
	}
}
