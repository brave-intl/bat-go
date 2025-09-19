//go:build integration

package skus

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	must "github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/datastore"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/skus/model"
	"github.com/brave-intl/bat-go/services/skus/skustest"

	"github.com/brave-intl/bat-go/services/skus/storage/repository"
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
	storage, _ := NewPostgres(
		repository.NewOrder(),
		repository.NewOrderItem(),
		repository.NewOrderPayHistory(),
		repository.NewIssuer(),
		"", false, "",
	)

	suite.storage = storage
}

func (suite *PostgresTestSuite) SetupTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (s *PostgresTestSuite) TearDownSuite(sn, tn string) {
	skustest.CleanDB(s.T(), s.storage.RawDB())
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

	pg := &Postgres{
		Postgres:        datastore.Postgres{DB: sqlx.NewDb(mockDB, "sqlmock")},
		orderRepo:       repository.NewOrder(),
		orderItemRepo:   repository.NewOrderItem(),
		orderPayHistory: repository.NewOrderPayHistory(),
	}

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

func (suite *PostgresTestSuite) TestGetOrderByExternalID() {
	ctx := context.Background()
	defer ctx.Done()

	// create an issuer and a paid order with one order item and a time limited v2 credential type.
	ctx = context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	o1, _ := createOrderAndIssuer(suite.T(), ctx, suite.storage, devFreeTimeLimitedV2)

	// add the external id to metadata
	err := suite.storage.AppendOrderMetadata(ctx, &o1.ID, "externalID", "my external id")
	suite.Require().NoError(err)

	// test out get by external id
	o2, err := suite.storage.GetOrderByExternalID("my external id")
	suite.Require().NoError(err)
	suite.Assert().NotNil(o2)

	if o2 != nil {
		suite.Assert().Equal(o2.ID.String(), o1.ID.String())
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

func (suite *PostgresTestSuite) TestSendSigningRequest_MultipleRow_Success() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx := context.Background()

	// Insert multiple messages for processing

	orderID := uuid.NewV4()

	for i := 0; i < signingRequestBatchSize+1; i++ {
		metadata := Metadata{
			ItemID:         uuid.NewV4(),
			OrderID:        orderID,
			IssuerID:       uuid.NewV4(),
			CredentialType: test.RandomString(),
		}

		associatedData, err := json.Marshal(metadata)
		suite.Require().NoError(err)

		requestID := uuid.NewV4()

		signingOrderRequest := SigningOrderRequest{
			RequestID: requestID.String(),
			Data: []SigningOrder{
				{
					IssuerType:     test.RandomString(),
					IssuerCohort:   defaultCohort,
					BlindedTokens:  []string{test.RandomString()},
					AssociatedData: associatedData,
				},
			},
		}

		err = suite.storage.InsertSigningOrderRequestOutbox(context.Background(), requestID, metadata.OrderID,
			metadata.ItemID, signingOrderRequest)
		suite.Require().NoError(err)
	}

	// Capture the messages that are picked up. We need to do this as the query
	// just picks up any non-processed messages.
	// We should only process the max batch size.

	var messagesItemID []uuid.UUID

	signingRequestWriter := NewMockSigningRequestWriter(ctrl)
	signingRequestWriter.EXPECT().
		WriteMessages(gomock.Any(), gomock.Len(signingRequestBatchSize)).
		Do(func(ctx context.Context, messages []SigningOrderRequestOutbox) {
			for _, message := range messages {
				messagesItemID = append(messagesItemID, message.ItemID)
			}
		}).
		Times(1).
		Return(nil)

	err := suite.storage.SendSigningRequest(ctx, signingRequestWriter)
	suite.Require().NoError(err)

	// Assert kafka mock was called with signing requests
	suite.Require().NotNil(messagesItemID)

	// Assert that all the messages picked up have been marked as processed

	qry, args, err := sqlx.In(`select order_id, submitted_at from signing_order_request_outbox where item_id IN (?)`,
		messagesItemID)
	suite.Require().NoError(err)

	type outboxMessage struct {
		OrderID     uuid.UUID  `db:"order_id"`
		SubmittedAt *time.Time `db:"submitted_at"`
	}

	var actual []outboxMessage

	err = suite.storage.RawDB().SelectContext(ctx, &actual, suite.storage.RawDB().Rebind(qry), args...)
	suite.Require().NoError(err)

	for _, s := range actual {
		suite.Assert().NotNil(s.SubmittedAt)
	}
}

func (suite *PostgresTestSuite) TestSendSigningRequest_NoRows() {
	ctrl := gomock.NewController(suite.T())
	defer ctrl.Finish()

	ctx := context.Background()

	// should not be called
	signingRequestWriter := NewMockSigningRequestWriter(ctrl)
	signingRequestWriter.EXPECT().
		WriteMessages(ctx, gomock.Any()).
		Return(nil).
		Times(0)

	err := suite.storage.SendSigningRequest(ctx, signingRequestWriter)
	suite.Require().NoError(err)
}

func (suite *PostgresTestSuite) TestInsertSignedOrderCredentials_SingleUse_Success() {
	ctx := context.Background()

	// create an issuer and a paid order with one order item and a single use credential type.
	ctx = context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devUserWalletVote})
	order, issuer := createOrderAndIssuer(suite.T(), ctx, suite.storage, devUserWalletVote)

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

	ctx, tx, rollback, commit, err := datastore.GetTx(ctx, suite.storage)
	defer rollback()

	err = suite.storage.InsertSignedOrderCredentialsTx(ctx, tx, signingOrderResult)
	suite.Require().NoError(err)

	err = commit()
	suite.Require().NoError(err)

	time.Sleep(time.Millisecond)

	actual, err := suite.storage.GetOrderCredsByItemID(order.ID, order.Items[0].ID, false)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.SignedCreds)
}

func (suite *PostgresTestSuite) TestInsertSignedOrderCredentials_TimeAwareV2_Success() {
	ctx := context.Background()

	// create an issuer and a paid order with one order item and a time limited v2 credential type.
	ctx = context.WithValue(context.Background(), appctx.WhitelistSKUsCTXKey, []string{devFreeTimeLimitedV2})
	order, issuer := createOrderAndIssuer(suite.T(), ctx, suite.storage, devFreeTimeLimitedV2)

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

	ctx, tx, rollback, commit, err := datastore.GetTx(ctx, suite.storage)
	defer rollback()

	err = suite.storage.InsertSignedOrderCredentialsTx(ctx, tx, signingOrderResult)
	suite.Require().NoError(err)

	err = commit()
	suite.Require().NoError(err)

	time.Sleep(time.Millisecond)

	actual, err := suite.storage.GetTimeLimitedV2OrderCredsByOrderItem(order.Items[0].ID)
	suite.Require().NoError(err)

	suite.Require().NotNil(actual)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, actual.Credentials[0].PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, actual.Credentials[0].BatchProof)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), actual.Credentials[0].SignedCreds)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].BlindedTokens), actual.Credentials[0].BlindedCreds)

	to, err := time.Parse(time.RFC3339, vTo)
	suite.Require().NoError(err)

	from, err := time.Parse(time.RFC3339, vFrom)
	suite.Require().NoError(err)

	suite.Assert().Equal(to, actual.Credentials[0].ValidTo)
	suite.Assert().Equal(from, actual.Credentials[0].ValidFrom)
}

func (suite *PostgresTestSuite) TestInsertSigningOrderRequestOutbox() {
	orderID := uuid.NewV4()
	itemID := uuid.NewV4()

	requestID := uuid.NewV4()

	signingOrderRequest := SigningOrderRequest{
		RequestID: requestID.String(),
		Data: []SigningOrder{
			{
				IssuerType:     test.RandomString(),
				IssuerCohort:   defaultCohort,
				BlindedTokens:  []string{test.RandomString()},
				AssociatedData: []byte{},
			},
		},
	}

	ctx := context.Background()

	err := suite.storage.InsertSigningOrderRequestOutbox(ctx, requestID, orderID, itemID, signingOrderRequest)
	suite.Require().NoError(err)

	signingOrderRequests, err := suite.storage.GetSigningOrderRequestOutboxByOrderItem(ctx, orderID, itemID)
	suite.Require().NoError(err)

	suite.Require().Len(signingOrderRequests, 1)

	suite.Assert().Equal(orderID, signingOrderRequests[0].OrderID)
	suite.Assert().Equal(itemID, signingOrderRequests[0].ItemID)

	var actual SigningOrderRequest
	err = json.Unmarshal(signingOrderRequests[0].Message, &actual)
	suite.Assert().NoError(err)
	suite.Assert().Equal(signingOrderRequest, actual)
}

func (suite *PostgresTestSuite) TestGetOutboxMovAvgDurationSeconds() {
	raw, err := json.Marshal(SigningOrderRequest{})
	suite.Require().NoError(err)

	ctx := context.Background()

	const q = `INSERT INTO signing_order_request_outbox (request_id, order_id, item_id, message_data, submitted_at, completed_at) VALUES ($1, $2, $3, $4, $5, $6)`

	subAt := time.Date(2023, time.August, 1, 1, 1, 1, 0, time.UTC)
	compAt := time.Date(2023, time.August, 1, 1, 1, 4, 0, time.UTC)

	for range 3 {
		_, err = suite.storage.RawDB().ExecContext(ctx, q, uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), raw, subAt, compAt)
		suite.Require().NoError(err)
	}

	actual, err := suite.storage.GetOutboxMovAvgDurationSeconds()
	suite.Require().NoError(err)

	suite.Assert().Equal(int64(3), actual)
}

//nolint:typecheck
func createOrderAndIssuer(t *testing.T, ctx context.Context, storage Datastore, sku ...string) (*Order, *Issuer) {
	var (
		svc        = &Service{}
		orderItems []OrderItem
		methods    []string
	)

	for _, s := range sku {
		orderItem, method, _, err := svc.CreateOrderItemFromMacaroon(ctx, s, 1)
		must.NoError(t, err)

		orderItems = append(orderItems, *orderItem)
		methods = append(methods, method...)
	}

	validFor := 3600 * time.Second * 24

	oreq := &model.OrderNew{
		MerchantID: test.RandomString(),
		Currency:   test.RandomString(),
		Status:     model.OrderStatusPaid,
		TotalPrice: decimal.NewFromInt(int64(test.RandomInt())),
		Location: sql.NullString{
			Valid:  true,
			String: test.RandomString(),
		},
		AllowedPaymentMethods: pq.StringArray(methods),
		ValidFor:              &validFor,
	}

	order, err := storage.CreateOrder(ctx, storage.RawDB(), oreq, orderItems)
	must.NoError(t, err)

	{
		err := storage.UpdateOrder(order.ID, OrderStatusPaid)
		must.NoError(t, err)
	}

	repo := repository.NewIssuer()
	issuer, err := repo.Create(ctx, storage.RawDB(), model.IssuerNew{
		MerchantID: model.MerchID,
		PublicKey:  test.RandomString(),
	})
	must.NoError(t, err)

	return order, issuer
}

// helper to setup a paid order, order items, issuer and insert time limited v2 order credentials
func (suite *PostgresTestSuite) createTimeLimitedV2OrderCreds(t *testing.T, ctx context.Context, sku ...string) []TimeAwareSubIssuedCreds {
	var (
		svc        = Service{}
		orderItems []OrderItem
		methods    []string
	)

	for _, s := range sku {
		orderItem, method, _, err := svc.CreateOrderItemFromMacaroon(ctx, s, 1)
		must.NoError(t, err)

		orderItems = append(orderItems, *orderItem)
		methods = append(methods, method...)
	}

	oreq := &model.OrderNew{
		MerchantID: test.RandomString(),
		Currency:   test.RandomString(),
		Status:     model.OrderStatusPaid,
		TotalPrice: decimal.NewFromInt(int64(test.RandomInt())),
		Location: sql.NullString{
			Valid:  true,
			String: test.RandomString(),
		},
		AllowedPaymentMethods: pq.StringArray(methods),
	}

	order, err := suite.storage.CreateOrder(ctx, suite.storage.RawDB(), oreq, orderItems)
	must.NoError(t, err)

	repo := repository.NewIssuer()

	issuer, err := repo.Create(ctx, suite.storage.RawDB(), model.IssuerNew{
		MerchantID: test.RandomString(),
		PublicKey:  test.RandomString(),
	})
	must.NoError(t, err)

	// create the time limited order credentials for each of the order items in our order
	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	validTo, err := time.Parse(time.RFC3339, to)
	must.NoError(t, err)

	from := time.Now().Local().Format(time.RFC3339)
	validFrom, err := time.Parse(time.RFC3339, from)
	must.NoError(t, err)

	signedCreds := jsonutils.JSONStringArray([]string{test.RandomString()})

	var orderCredentials []TimeAwareSubIssuedCreds

	_, tx, rollback, commit, err := datastore.GetTx(ctx, suite.storage)
	suite.Require().NoError(err)

	defer rollback()

	for _, orderItem := range order.Items {
		tlv2 := TimeAwareSubIssuedCreds{
			ItemID:       orderItem.ID,
			OrderID:      order.ID,
			IssuerID:     issuer.ID,
			BlindedCreds: []string{test.RandomString()},
			SignedCreds:  signedCreds,
			BatchProof:   test.RandomString(),
			PublicKey:    issuer.PublicKey,
			ValidTo:      validTo,
			ValidFrom:    validFrom,
		}

		err := suite.storage.InsertTimeLimitedV2OrderCredsTx(ctx, tx, tlv2)
		must.NoError(t, err)

		orderCredentials = append(orderCredentials, tlv2)
	}

	{
		err := commit()
		suite.Require().NoError(err)
	}

	return orderCredentials
}

// helper to setup a paid order, order items, issuer and insert unsigned order credentials
func (suite *PostgresTestSuite) createOrderCreds(t *testing.T, ctx context.Context, sku ...string) []*OrderCreds {
	var (
		svc        = Service{}
		orderItems []OrderItem
		methods    []string
	)

	for _, s := range sku {
		orderItem, method, _, err := svc.CreateOrderItemFromMacaroon(ctx, s, 1)
		must.NoError(t, err)

		orderItems = append(orderItems, *orderItem)
		methods = append(methods, method...)
	}

	oreq := &model.OrderNew{
		MerchantID: test.RandomString(),
		Currency:   test.RandomString(),
		Status:     model.OrderStatusPaid,
		TotalPrice: decimal.NewFromInt(int64(test.RandomInt())),
		Location: sql.NullString{
			Valid:  true,
			String: test.RandomString(),
		},
		AllowedPaymentMethods: pq.StringArray(methods),
	}

	order, err := suite.storage.CreateOrder(ctx, suite.storage.RawDB(), oreq, orderItems)
	must.NoError(t, err)

	pk := test.RandomString()

	repo := repository.NewIssuer()

	issuer, err := repo.Create(ctx, suite.storage.RawDB(), model.IssuerNew{
		MerchantID: test.RandomString(),
		PublicKey:  pk,
	})
	must.NoError(t, err)

	signedCreds := jsonutils.JSONStringArray([]string{test.RandomString()})

	var orderCredentials []*OrderCreds

	_, tx, rollback, commit, err := datastore.GetTx(ctx, suite.storage)
	suite.Require().NoError(err)

	defer rollback()

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

		err := suite.storage.InsertOrderCredsTx(ctx, tx, oc)
		must.NoError(t, err)

		orderCredentials = append(orderCredentials, oc)
	}

	{
		err := commit()
		suite.Require().NoError(err)
	}

	return orderCredentials
}
