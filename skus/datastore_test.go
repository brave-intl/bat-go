//go:build integration

package skus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/skus/skustest"
	timeutils "github.com/brave-intl/bat-go/utils/time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/skus"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	"github.com/brave-intl/bat-go/utils/ptr"
	"github.com/brave-intl/bat-go/utils/test"
	"github.com/golang/mock/gomock"
	"github.com/jmoiron/sqlx"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

const (
	devBraveSearchPremiumYearTimeLimited  = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDM2aWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bS15ZWFyIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS1zZWFyY2gtYWRmcmVlCjAwMTRjaWQgcHJpY2U9MzAuMDAKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzM2NpZCBkZXNjcmlwdGlvbj1QcmVtaXVtIGFjY2VzcyB0byBCcmF2ZSBTZWFyY2gKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMVkKMDAxZWNpZCBpc3N1YW5jZV9pbnRlcnZhbD1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9KelNldnlaTTVpQlNyZiIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSm9YdkZIb2YyMGJwaEc2eUg2a1FpUEciLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgfSNU9u0uAbGm1Vi8dKoa9hcK71VeMzGUWq77io6sJgUK"
	devBraveFirewallVPNPremiumTimeLimited = "MDAyMGxvY2F0aW9uIHZwbi5icmF2ZS5zb2Z0d2FyZQowMDM3aWRlbnRpZmllciBicmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyN2NpZCBza3U9YnJhdmUtZmlyZXdhbGwtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMjljaWQgZGVzY3JpcHRpb249QnJhdmUgRmlyZXdhbGwgKyBWUE4KMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9LMWM4VzNvTTRtVXNHdyIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSk5ZdU5Ib2YyMGJwaEc2QnZnZVlFbnQiLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgZoDg2iXb36IocwS9/MZnvP5Hk2NfAdJ6qMs0kBSyinUK"
)

type PostgresTestSuite struct {
	suite.Suite
	storage skus.Datastore
}

func TestPostgresTestSuite(t *testing.T) {
	suite.Run(t, new(PostgresTestSuite))
}

func (suite *PostgresTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	storage, _ := skus.NewPostgres("", false, "")
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
	pg := &skus.Postgres{Postgres: grantserver.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")}}

	// setup inputs
	merchantID := uuid.NewV4()
	ctx, pagination, err := inputs.NewPagination(ctx, "/?page=2&items=50&order=id.asc&order=createdAt.desc", new(skus.Transaction))
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
	order := suite.createOrderAndCredentials(ctx, devBraveFirewallVPNPremiumTimeLimited)

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
	order := suite.createOrderAndCredentials(ctx, devBraveFirewallVPNPremiumTimeLimited, devBraveSearchPremiumYearTimeLimited) // insert initial order creds

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
	orderItems := make(map[uuid.UUID]skus.OrderItem)
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
	order := suite.createOrderAndCredentials(ctx, devBraveFirewallVPNPremiumTimeLimited)

	publicKey := test.RandomString()

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	vFrom := time.Now().Local().Format(time.RFC3339)
	vTo := time.Now().Local().Add(time.Hour).Format(time.RFC3339)

	signingOrderResult := &skus.SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []skus.SignedOrder{
			{
				PublicKey:      publicKey,
				Proof:          test.RandomString(),
				Status:         skus.SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &skus.UnionNullString{"string": vTo},
				ValidFrom:      &skus.UnionNullString{"string": vFrom},
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := skus.NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	go func() {
		err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)
		fmt.Println(err)
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
	order := suite.createOrderAndCredentials(ctx, devBraveFirewallVPNPremiumTimeLimited)

	publicKey := test.RandomString()

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := &skus.SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []skus.SignedOrder{
			{
				PublicKey:      publicKey,
				Proof:          test.RandomString(),
				Status:         skus.SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := skus.NewMockOrderCredentialsWorker(ctrl)
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
	order := suite.createOrderAndCredentials(ctx, devBraveFirewallVPNPremiumTimeLimited)

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	ad, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := &skus.SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []skus.SignedOrder{
			{
				Status:         skus.SignedOrderStatusError,
				AssociatedData: ad,
			},
		},
	}

	orderCredentialsWorker := skus.NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)

	suite.Assert().EqualError(err, fmt.Sprintf("error signing order creds for orderID %s itemID %s status %s",
		associatedData["order_id"], associatedData["item_id"], skus.SignedOrderStatusError.String()))
}

// helper to setup a paid order, order items, issuer and insert unsigned order credentials
func (suite *PostgresTestSuite) createOrderAndCredentials(ctx context.Context, sku ...string) *skus.Order {
	service := skus.Service{}

	var orderItems []skus.OrderItem
	var methods skus.Methods

	for _, s := range sku {
		orderItem, method, _, err := service.CreateOrderItemFromMacaroon(ctx, s, 1)
		suite.Require().NoError(err)
		orderItems = append(orderItems, *orderItem)
		methods = append(methods, *method...)
	}

	order, err := suite.storage.CreateOrder(decimal.NewFromInt32(int32(test.RandomInt())), test.RandomString(), skus.OrderStatusPaid,
		test.RandomString(), test.RandomString(), nil, orderItems, &methods)
	suite.Require().NoError(err)

	// create issuer
	pk := test.RandomString()

	issuer := &skus.Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  pk,
	}

	issuer, err = suite.storage.InsertIssuer(issuer)
	suite.Require().NoError(err)

	tx, err := suite.storage.RawDB().BeginTxx(ctx, nil)
	suite.Require().NoError(err)

	defer func() {
		_ = tx.Rollback()
	}()

	// insert order creds
	for _, orderItem := range order.Items {
		oc := &skus.OrderCreds{
			ID:           orderItem.ID, // item_id
			OrderID:      order.ID,
			IssuerID:     issuer.ID,
			BlindedCreds: nil,
			BatchProof:   ptr.FromString(test.RandomString()),
			PublicKey:    ptr.FromString(pk),
		}
		err = suite.storage.InsertOrderCreds(ctx, tx, oc)
		suite.Require().NoError(err)
	}

	err = tx.Commit()
	suite.Require().NoError(err)

	return order
}
