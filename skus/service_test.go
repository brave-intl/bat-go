//go:build integration

package skus

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/skus/skustest"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/jsonutils"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/brave-intl/bat-go/utils/test"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ServiceTestSuite struct {
	suite.Suite
	storage Datastore
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}

func (suite *ServiceTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	suite.storage, _ = NewPostgres("", false, "")
}

func (suite *ServiceTestSuite) AfterTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

func (suite *ServiceTestSuite) TestRunStoreSignedOrderCredentialsJob() {
	ctx := context.Background()

	env := os.Getenv("ENV")
	ctx = context.WithValue(ctx, appctx.EnvironmentCTXKey, env)

	// create paid order and insert order creds
	ctx = context.WithValue(ctx, appctx.WhitelistSKUsCTXKey, []string{devBraveSearchPremiumYearTimeLimited})
	pg := PostgresTestSuite{storage: suite.storage}
	order := pg.createOrderAndCredentials(suite.T(), ctx, devBraveSearchPremiumYearTimeLimited)

	// setup kafka and write expected signed creds to topic. Overwrite topics so fresh for each test
	kafkaSignedOrderCredsTopic = test.RandomString()
	kafkaOrderCredsSignedRequestReaderGroupID = test.RandomString()
	ctx = skustest.SetupKafka(suite.T(), ctx, kafkaSignedOrderCredsTopic)

	associatedData := make(map[string]string)
	associatedData["order_id"] = order.ID.String()
	associatedData["item_id"] = order.Items[0].ID.String()

	b, err := json.Marshal(associatedData)
	suite.Require().NoError(err)

	signingOrderResult := SigningOrderResult{
		RequestID: uuid.NewV4().String(),
		Data: []SignedOrder{
			{
				PublicKey:      test.RandomString(),
				Proof:          test.RandomString(),
				Status:         SignedOrderStatusOk,
				SignedTokens:   []string{test.RandomString()},
				ValidTo:        &UnionNullString{"string": time.Now().Format(time.RFC3339)},
				ValidFrom:      &UnionNullString{"string": time.Now().Add(time.Hour).Format(time.RFC3339)},
				AssociatedData: b,
			},
		},
	}
	writeSigningOrderResultMessage(suite.T(), ctx, signingOrderResult, kafkaSignedOrderCredsTopic)

	// act
	go func() {
		service, _ := InitService(ctx, suite.storage, nil)
		_, _ = service.RunStoreSignedOrderCredentialsJob(ctx)
	}()

	time.Sleep(1 * time.Second)

	// assert
	actual, err := suite.storage.GetOrderTimeLimitedV2CredsByItemID(order.ID, order.Items[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(actual)

	suite.Assert().Equal(order.ID, actual.Credentials[0].OrderID)
	suite.Assert().Equal(jsonutils.JSONStringArray(signingOrderResult.Data[0].SignedTokens), *actual.Credentials[0].SignedCreds)
	suite.Assert().Equal(signingOrderResult.Data[0].PublicKey, *actual.Credentials[0].PublicKey)
	suite.Assert().Equal(signingOrderResult.Data[0].Proof, *actual.Credentials[0].BatchProof)
	suite.Assert().NotEmpty(*actual.Credentials[0].ValidTo)
	suite.Assert().NotEmpty(*actual.Credentials[0].ValidFrom)
}

func TestCredChunkFn(t *testing.T) {
	// Jan 1, 2021
	issued := time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC)

	// 1 day
	day, err := timeutils.ParseDuration("P1D")
	if err != nil {
		t.Errorf("failed to parse 1 day: %s", err.Error())
	}

	// 1 month
	mo, err := timeutils.ParseDuration("P1M")
	if err != nil {
		t.Errorf("failed to parse 1 month: %s", err.Error())
	}

	this, next := credChunkFn(*day)(issued)
	if this.Day() != 20 {
		t.Errorf("day - the next day should be 2")
	}
	if this.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}
	if next.Day() != 21 {
		t.Errorf("day - the next day should be 2")
	}
	if next.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}

	this, next = credChunkFn(*mo)(issued)
	if this.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if this.Month() != 1 {
		t.Errorf("mo - the next month should be 2")
	}
	if next.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if next.Month() != 2 {
		t.Errorf("mo - the next month should be 2")
	}
}

func writeSigningOrderResultMessage(t *testing.T, ctx context.Context, signingOrderResult SigningOrderResult, topic string) {
	codec, err := goavro.NewCodec(signingOrderResultSchema)
	assert.NoError(t, err)

	textual, err := json.Marshal(signingOrderResult)
	assert.NoError(t, err)

	native, _, err := codec.NativeFromTextual(textual)
	assert.NoError(t, err)

	binary, err := codec.BinaryFromNative(nil, native)
	assert.NoError(t, err)

	kafkaWriter, _, err := kafkautils.InitKafkaWriter(ctx, "")
	assert.NoError(t, err)

	err = kafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(signingOrderResult.RequestID),
		Value: binary,
	})
	assert.NoError(t, err)
}
