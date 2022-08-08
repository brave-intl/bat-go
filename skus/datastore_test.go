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

func (suite *PostgresTestSuite) SetupTest() {
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

func (suite *PostgresTestSuite) TestGetTimeLimitedV2OrderCredsByOrder_Success() {
	env := os.Getenv("ENV")
	ctx := context.WithValue(context.Background(), appctx.EnvironmentCTXKey, env)

	// create paid order with two order items
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited,
		devBraveSearchPremiumYearTimeLimited})

	orderCredentials := suite.createTimeLimitedV2OrderCreds(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited,
		devBraveSearchPremiumYearTimeLimited)

	// create an unrelated order which should not be returned
	_ = suite.createTimeLimitedV2OrderCreds(suite.T(), ctx, devBraveSearchPremiumYearTimeLimited)

	// both order items have same orderID so can use the first element to retrieve all order creds
	actual, err := suite.storage.GetTimeLimitedV2OrderCredsByOrder(orderCredentials[0].OrderID)
	suite.Require().NoError(err)

	suite.Assert().Equal(orderCredentials[0].OrderID, actual.OrderID)
	suite.Assert().Equal(orderCredentials[0].IssuerID, actual.IssuerID)
	// this will contain all the order creds for each of the order items
	suite.Assert().ElementsMatch(orderCredentials, actual.Credentials)
}

func (suite *PostgresTestSuite) TestGetTimeLimitedV2OrderCredsByOrderItem_Success() {
	env := os.Getenv("ENV")
	ctx := context.WithValue(context.Background(), appctx.EnvironmentCTXKey, env)

	// create paid order with two order items
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveFirewallVPNPremiumTimeLimited,
		devBraveSearchPremiumYearTimeLimited})

	orderCredentials := suite.createTimeLimitedV2OrderCreds(suite.T(), ctx, devBraveFirewallVPNPremiumTimeLimited,
		devBraveSearchPremiumYearTimeLimited)

	// create an unrelated order which should not be returned
	_ = suite.createTimeLimitedV2OrderCreds(suite.T(), ctx, devBraveSearchPremiumYearTimeLimited)

	// retrieve the first order credential from our newly created order items
	actual, err := suite.storage.GetTimeLimitedV2OrderCredsByOrderItem(orderCredentials[0].ItemID)
	suite.Require().NoError(err)

	suite.Assert().Equal(orderCredentials[0].OrderID, actual.OrderID)
	suite.Assert().Equal(orderCredentials[0].IssuerID, actual.IssuerID)

	// credentials should only contain the order cred for the first item we retrieved
	suite.Assert().Equal(1, len(actual.Credentials))
	suite.Assert().ElementsMatch(orderCredentials[0:1], actual.Credentials)
}

func (suite *PostgresTestSuite) TestSendSigningRequest_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx := context.Background()

	metadata := Metadata{
		ItemID:         uuid.NewV4(),
		OrderID:        uuid.NewV4(),
		IssuerID:       uuid.NewV4(),
		CredentialType: test.RandomString(),
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	signingOrderRequests := []SigningOrderRequest{
		{
			RequestID: test.RandomString(),
			Data: []SigningOrder{
				{
					IssuerType:     test.RandomString(),
					IssuerCohort:   defaultCohort,
					BlindedTokens:  []string{test.RandomString()},
					AssociatedData: associatedData,
				},
			},
		},
	}

	signingRequestWriter := NewMockSigningRequestWriter(ctrl)
	signingRequestWriter.EXPECT().WriteMessage(ctx, gomock.Any()).
		Return(nil).
		AnyTimes()

	err = suite.storage.InsertSigningOrderRequestOutbox(context.Background(), metadata.OrderID, signingOrderRequests)
	suite.Require().NoError(err)

	err = suite.storage.SendSigningRequest(ctx, signingRequestWriter)
	suite.Require().NoError(err)

	// Use OutboxMessage as we don't want to expose certain fields in production code
	type OutboxMessage struct {
		ProcessedAt time.Time `db:"processed_at"`
		OrderID     uuid.UUID `db:"order_id"`
	}

	var om OutboxMessage
	err = suite.storage.RawDB().GetContext(ctx, &om,
		`select order_id, processed_at from signing_order_request_outbox where order_id = $1`, metadata.OrderID)
	suite.Require().NoError(err)

	suite.Assert().Equal(metadata.OrderID, om.OrderID)
	suite.Assert().NotNil(om.ProcessedAt)
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_SingleUse_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx := context.Background()
	defer ctx.Done()

	// create an issuer and a paid order with one order item and a single use credential type.
	ctx = context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devUserWalletVote})
	order, issuer := suite.createOrderAndIssuer(suite.T(), ctx, devUserWalletVote)

	metadata := Metadata{
		ItemID:         order.Items[0].ID,
		OrderID:        order.ID,
		IssuerID:       issuer.ID,
		CredentialType: order.Items[0].CredentialType,
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      issuer.PublicKey,
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				BlindedTokens:  []string{test.RandomString()},
				SignedTokens:   []string{test.RandomString()},
				AssociatedData: associatedData,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	go func() {
		_ = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)
	}()

	time.Sleep(time.Millisecond)

	actual, err := suite.storage.GetOrderCredsByItemID(order.ID, order.Items[0].ID, false)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.SignedCreds)
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_TimeAwareV2_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx := context.Background()
	defer ctx.Done()

	// create an issuer and a paid order with one order item and a time limited v2 credential type.
	ctx = context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	order, issuer := suite.createOrderAndIssuer(suite.T(), ctx, devFreeTimeLimitedV2)

	metadata := Metadata{
		ItemID:         order.Items[0].ID,
		OrderID:        order.ID,
		IssuerID:       issuer.ID,
		CredentialType: order.Items[0].CredentialType,
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	vFrom := time.Now().Local().Format(time.RFC3339)
	vTo := time.Now().Local().Add(time.Hour).Format(time.RFC3339)

	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      issuer.PublicKey,
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				BlindedTokens:  []string{test.RandomString()},
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &UnionNullString{"string": vTo},
				ValidFrom:      &UnionNullString{"string": vFrom},
				AssociatedData: associatedData,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	go func() {
		_ = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)
	}()

	time.Sleep(time.Millisecond)

	actual, err := suite.storage.GetTimeLimitedV2OrderCredsByOrderItem(order.Items[0].ID)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, actual.Credentials[0].PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, actual.Credentials[0].BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), actual.Credentials[0].SignedCreds)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].BlindedTokens), actual.Credentials[0].BlindedCreds)

	to, err := timeutils.ParseStringToTime(&vTo)
	suite.Require().NoError(err)

	from, err := timeutils.ParseStringToTime(&vFrom)
	suite.Require().NoError(err)

	suite.Assert().Equal(*to, actual.Credentials[0].ValidTo)
	suite.Assert().Equal(*from, actual.Credentials[0].ValidFrom)
}

func (suite *PostgresTestSuite) TestStoreSignedOrderCredentials_SignedOrderStatus_Error() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	// create free order and order items
	ctx := context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	order, issuer := suite.createOrderAndIssuer(suite.T(), ctx, devFreeTimeLimitedV2)

	metadata := Metadata{
		ItemID:         order.Items[0].ID,
		OrderID:        order.ID,
		IssuerID:       issuer.ID,
		CredentialType: order.Items[0].CredentialType,
	}

	associatedData, err := json.Marshal(metadata)
	suite.Require().NoError(err)

	// created a failed signing result
	signingOrderResult := &SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				Status:         SignedOrderStatusError,
				AssociatedData: associatedData,
			},
		},
	}

	orderCredentialsWorker := NewMockOrderCredentialsWorker(ctrl)
	orderCredentialsWorker.EXPECT().
		FetchSignedOrderCredentials(ctx).
		Return(signingOrderResult, nil).
		AnyTimes()

	err = suite.storage.StoreSignedOrderCredentials(ctx, orderCredentialsWorker)

	suite.Assert().EqualError(err, fmt.Sprintf("error signing order creds for orderID %s itemID %s issuerID %s status %s",
		metadata.OrderID, metadata.ItemID, metadata.IssuerID, SignedOrderStatusError.String()))
}

// helper to set up a paid order, order items and issuer.
func (suite *PostgresTestSuite) createOrderAndIssuer(t *testing.T, ctx context.Context, sku ...string) (*Order, *Issuer) {
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
	issuer := &Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  test.RandomString(),
	}
	issuer, err = suite.storage.InsertIssuer(issuer)
	assert.NoError(t, err)

	return order, issuer
}

// helper to setup a paid order, order items, issuer and insert time limited v2 order credentials
func (suite *PostgresTestSuite) createTimeLimitedV2OrderCreds(t *testing.T, ctx context.Context, sku ...string) []TimeAwareSubIssuedCreds {
	// create the order and the order items from our skus
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
	issuer := &Issuer{
		MerchantID: test.RandomString(),
		PublicKey:  test.RandomString(),
	}
	issuer, err = suite.storage.InsertIssuer(issuer)
	assert.NoError(t, err)

	// create the time limited order credentials for each of the order items in our order
	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	validTo, err := timeutils.ParseStringToTime(&to)
	assert.NoError(t, err)

	from := time.Now().Local().Format(time.RFC3339)
	validFrom, err := timeutils.ParseStringToTime(&from)
	assert.NoError(t, err)

	signedCreds := jsonutils.JSONStringArray([]string{test.RandomString()})

	var orderCredentials []TimeAwareSubIssuedCreds

	for _, orderItem := range order.Items {
		tlv2 := TimeAwareSubIssuedCreds{
			ItemID:       orderItem.ID,
			OrderID:      order.ID,
			IssuerID:     issuer.ID,
			BlindedCreds: []string{test.RandomString()},
			SignedCreds:  signedCreds,
			BatchProof:   test.RandomString(),
			PublicKey:    issuer.PublicKey,
			ValidTo:      *validTo,
			ValidFrom:    *validFrom,
		}

		err = suite.storage.InsertTimeLimitedV2OrderCreds(ctx, tlv2)
		assert.NoError(t, err)

		orderCredentials = append(orderCredentials, tlv2)
	}

	return orderCredentials
}

// helper to setup a paid order, order items, issuer and insert unsigned order credentials
func (suite *PostgresTestSuite) createOrderCreds(t *testing.T, ctx context.Context, sku ...string) []*OrderCreds {
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

	signedCreds := jsonutils.JSONStringArray([]string{test.RandomString()})

	var orderCredentials []*OrderCreds

	// insert order creds
	for _, orderItem := range order.Items {
		oc := &OrderCreds{
			ID:           orderItem.ID, // item_id
			OrderID:      order.ID,
			IssuerID:     issuer.ID,
			BlindedCreds: []string{test.RandomString()},
			SignedCreds:  &signedCreds,
			BatchProof:   ptr.FromString(test.RandomString()),
			PublicKey:    ptr.FromString(pk),
		}
		err = suite.storage.InsertOrderCreds(ctx, oc)
		assert.NoError(t, err)
		orderCredentials = append(orderCredentials, oc)
	}

	return orderCredentials
}
