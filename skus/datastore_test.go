//go:build integration

package skus

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/skus/skustest"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/test"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/golang/mock/gomock"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PostgresTestSuite struct {
	suite.Suite
	storage Datastore
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

func (suite *PostgresTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	storage, _ := NewPostgres("", false, "")
	suite.storage = storage
}

func (suite *PostgresTestSuite) AfterTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

func TestGetPagedMerchantTransactions(t *testing.T) {
	ctx := context.Background()
	// setup mock DB we will inject into our pg
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Errorf("failed to create a sql mock: %s", err)
	}
	defer func() {
		if err := mockDB.Close(); err != nil {
			if !strings.Contains(err.Error(), "all expectations were already fulfilled") {
				t.Errorf("failed to close the mock database: %s", err)
			}
		}
	}()
	// inject our mock db into our postgres
	pg := &Postgres{Postgres: grantserver.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")}}

	// setup inputs
	merchantID := uuid.NewV4()
	ctx, pagination, err := inputs.NewPagination(ctx, "/?page=2&items=50&order=id.asc&order=createdAt.desc", new(Transaction))
	if err != nil {
		t.Errorf("failed to create pagination: %s\n", err)
	}

	// setup expected mocks
	countRows := sqlmock.NewRows([]string{"total"}).AddRow(3)
	mock.ExpectQuery(`
			SELECT (.+) as total
			FROM transactions as t
				INNER JOIN orders as o ON o.id = t.order_id
			WHERE (.+)`).WithArgs(merchantID).WillReturnRows(countRows)

	transactionUUIDs := []uuid.UUID{uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	orderUUIDs := []uuid.UUID{uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	createdAt := []time.Time{time.Now(), time.Now().Add(time.Second * 5), time.Now().Add(time.Second * 10)}

	getRows := sqlmock.NewRows(
		[]string{"id", "order_id", "created_at", "updated_at",
			"external_transaction_id", "status", "currency", "kind", "amount"}).
		AddRow(transactionUUIDs[0], orderUUIDs[0], createdAt[0], createdAt[0], "", "pending", "BAT", "subscription", 10).
		AddRow(transactionUUIDs[1], orderUUIDs[1], createdAt[1], createdAt[1], "", "pending", "BAT", "subscription", 10).
		AddRow(transactionUUIDs[2], orderUUIDs[2], createdAt[2], createdAt[2], "", "pending", "BAT", "subscription", 10)

	mock.ExpectQuery(`
			SELECT (.+)
			FROM transactions as t
				INNER JOIN orders as o ON o.id = t.order_id
			WHERE o.merchant_id = (.+)
			 ORDER BY (.+) OFFSET (.+) FETCH NEXT (.+)`).WithArgs(merchantID).
		WillReturnRows(getRows)

	// call function under test with inputs
	transactions, c, err := pg.GetPagedMerchantTransactions(ctx, merchantID, pagination)

	// test assertions
	if err != nil {
		t.Errorf("failed to get paged merchant transactions: %s\n", err)
	}
	if len(*transactions) != 3 {
		t.Errorf("should have seen 3 transactions: %+v\n", transactions)
	}
	if c != 3 {
		t.Errorf("should have total count of 3 transactions: %d\n", c)
	}
}

func (suite *PostgresTestSuite) TestGetOrderTimeLimitedV2CredsByItemID_Success() {
	ctx := context.Background()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// create paid order with unsigned creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited})
	order := suite.createOrderAndCredentials(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited)

	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	validTo, err := timeutils.ParseStringToTime(&to)
	suite.Require().NoError(err)

	from := time.Now().Local().Format(time.RFC3339)
	validFrom, err := timeutils.ParseStringToTime(&from)
	suite.Require().NoError(err)

	// sign creds and add metadata
	signedCreds := &jsonutils.JSONStringArray{test.RandomString()}

	_, err = suite.storage.RawDB().Exec(`update order_creds set signed_creds = $1, valid_from = $2, valid_to = $3
												where order_id = $4 and item_id = $5`, signedCreds, validFrom, validTo,
		order.ID, order.Items[0].ID)
	suite.Require().NoError(err)

	// assert
	timeLimitedV2Creds, err := suite.storage.GetOrderTimeLimitedV2CredsByItemID(order.ID, order.Items[0].ID)
	suite.Require().NoError(err)

	suite.Assert().Equal(1, len(timeLimitedV2Creds.Credentials))
	suite.Assert().Equal(order.ID, timeLimitedV2Creds.OrderID)
	suite.Assert().Equal(order.Items[0].ID, timeLimitedV2Creds.Credentials[0].ItemID)
	suite.Assert().Equal(*validTo, *timeLimitedV2Creds.Credentials[0].ValidTo)
	suite.Assert().Equal(*validFrom, *timeLimitedV2Creds.Credentials[0].ValidFrom)
}

func (suite *PostgresTestSuite) TestGetOrderTimeLimitedV2Creds_Success() {
	ctx := context.Background()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// create paid order with unsigned creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited, devBraveSearchPremiumYearTimeLimited})
	order := suite.createOrderAndCredentials(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited, devBraveSearchPremiumYearTimeLimited) // insert initial order creds

	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	validTo, err := timeutils.ParseStringToTime(&to)
	suite.Require().NoError(err)

	from := time.Now().Local().Format(time.RFC3339)
	validFrom, err := timeutils.ParseStringToTime(&from)
	suite.Require().NoError(err)

	signedCreds := &jsonutils.JSONStringArray{test.RandomString()}

	_, err = suite.storage.RawDB().Exec(`update order_creds set signed_creds = $1, valid_from = $2, valid_to = $3
												where order_id = $4`, signedCreds, validFrom, validTo, order.ID)
	suite.Require().NoError(err)

	// add to map so we can compare the correct items
	orderItems := make(map[uuid.UUID]OrderItem)
	orderItems[order.Items[0].ID] = order.Items[0]
	orderItems[order.Items[1].ID] = order.Items[1]

	// assert

	timeLimitedV2Creds, err := suite.storage.GetOrderTimeLimitedV2Creds(order.ID)
	suite.Require().NoError(err)

	suite.Assert().Equal(2, len(*timeLimitedV2Creds))

	for _, actual := range *timeLimitedV2Creds {
		expected, ok := orderItems[actual.Credentials[0].ItemID]
		suite.Require().True(ok)

		suite.Require().Equal(1, len(actual.Credentials))
		suite.Require().Equal(expected.OrderID, actual.OrderID)
		suite.Assert().Equal(expected.ID, actual.Credentials[0].ItemID)
		suite.Assert().Equal(*validTo, *actual.Credentials[0].ValidTo)
		suite.Assert().Equal(*validFrom, *actual.Credentials[0].ValidFrom)
	}
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_TimeAware_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create paid order with unsigned creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited})
	order := suite.createOrderAndCredentials(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited)

	publicKey := test.RandomString()

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	vFrom := time.Now().Local().Format(time.RFC3339)
	vTo := time.Now().Local().Add(time.Hour).Format(time.RFC3339)

	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      publicKey,
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &UnionNullString{"string": vTo},
				ValidFrom:      &UnionNullString{"string": vFrom},
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	go func() {
		err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)
		suite.Require().NoError(err)
	}()

	time.Sleep(2 * time.Second)

	actual, err := suite.storage.GetOrderTimeLimitedV2CredsByItemID(order.ID, order.Items[0].ID)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.Credentials[0].PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.Credentials[0].BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.Credentials[0].SignedCreds)

	to, err := timeutils.ParseStringToTime(&vTo)
	suite.Require().NoError(err)

	from, err := timeutils.ParseStringToTime(&vFrom)
	suite.Require().NoError(err)

	suite.Assert().Equal(*to, *actual.Credentials[0].ValidTo)
	suite.Assert().Equal(*from, *actual.Credentials[0].ValidFrom)

	ctx.Done()
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_SingleUse_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create paid order with unsigned creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited})
	order := suite.createOrderAndCredentials(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited)

	publicKey := test.RandomString()

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      publicKey,
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	go func() {
		err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)
		suite.Require().NoError(err)
	}()

	time.Sleep(time.Millisecond)

	actual, err := suite.storage.GetOrderCredsByItemID(order.ID, order.Items[0].ID, false)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.SignedCreds)

	ctx.Done()
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_SignedOrderStatus_Error() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create paid order with unsigned creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited})
	order := suite.createOrderAndCredentials(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited)

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				Status:         SignedOrderStatusError,
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)

	suite.Assert().EqualError(err, fmt.Sprintf("error signing order creds for orderID %s itemID %s status %s",
		associatedData["order_id"], associatedData["item_id"], SignedOrderStatusError.String()))
}

// helper to setup a paid order, order items, issuer and insert unsigned order credentials
func (suite *PostgresTestSuite) createOrderAndCredentials(t *testing.T, ctx context.Context, sku ...string) *Order {
	service := Service{}
	var orderItems []OrderItem
	var methods Methods

	for _, s := range sku {
		orderItem, method, _, err := service.CreateOrderItemFromMacaroon(ctx, s, 1)
		assert.NoError(t, err)
		orderItems = append(orderItems, *orderItem)
		methods = append(methods, *method...)
	}

	order, err := suite.storage.CreateOrder(decimal.NewFromInt32(int32(test.RandomInt())), test.RandomString(), OrderStatusPaid,
		test.RandomString(), test.RandomString(), nil, orderItems, &methods)
	assert.NoError(t, err)

	// create issuer
	pk := test.RandomString()

	issuer := &Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  pk,
	}

	issuer, err = suite.storage.InsertIssuer(issuer)
	assert.NoError(t, err)

	tx, err := suite.storage.RawDB().BeginTxx(ctx, nil)
	assert.NoError(t, err)

	defer func() {
		_ = tx.Rollback()
	}()

	// insert order creds
	for _, orderItem := range order.Items {
		oc := &OrderCreds{
			ID:           orderItem.ID, // item_id
			OrderID:      order.ID,
			IssuerID:     issuer.ID,
			BlindedCreds: nil,
			BatchProof:   ptr.FromString(test.RandomString()),
			PublicKey:    ptr.FromString(pk),
		}
		err = suite.storage.InsertOrderCreds(ctx, tx, oc)
		assert.NoError(t, err)
	}

	err = tx.Commit()
	assert.NoError(t, err)

	return order
}
