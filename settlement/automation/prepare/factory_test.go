//go:build integration
// +build integration

package prepare_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement/automation/prepare"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	"github.com/brave-intl/bat-go/utils/logging"
	testutils "github.com/brave-intl/bat-go/utils/test"

	"github.com/brave-intl/bat-go/utils/ptr"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type PrepareTestSuite struct {
	suite.Suite
}

func TestPrepareTestSuite(t *testing.T) {
	suite.Run(t, new(PrepareTestSuite))
}

func (suite *PrepareTestSuite) SetupTest() {
	test.StreamsTearDown(suite.T())
}

func (suite *PrepareTestSuite) TestPrepare_Grants() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL, redisUsername, redisPassword)
	suite.Require().NoError(err)

	// stub payment service with expectedTransactions responses
	server := suite.stubPrepareEndpoint()
	defer server.Close()

	paymentURL := server.URL

	// create and send messages to prepare stream
	messages := make(map[string]event.Message)
	for i := 0; i < 5; i++ {
		transaction := payment.Transaction{
			IdempotencyKey: uuid.NewV4(),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
		}

		body, err := json.Marshal(transaction)
		suite.NoError(err)

		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.Grants,
			Body:      string(body),
		}

		messages[message.ID.String()] = message

		err = redis.Send(context.Background(), message, event.PrepareStream)
		suite.Require().NoError(err)
	}

	_, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	hexPrivateKey := hex.EncodeToString(privateKey)

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, redisUsername)
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, redisPassword)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, hexPrivateKey)
	ctx, done := context.WithTimeout(ctx, 10*time.Second)

	// start prepare consumer
	go prepare.StartConsumer(ctx) // nolint

	// assert message has been processed. once ads messages are consumed by prepare
	// these should be routed to the submit stream
	actualC := make(chan event.Message, len(messages))
	// start a test consumer to read from submit stream
	go test.StartTestBatchConsumer(suite.T(), ctx, redis, event.SubmitStream, actualC)

	for i := 0; i < len(messages); i++ {
		actual := <-actualC
		expected, ok := messages[actual.ID.String()]
		suite.True(ok)
		suite.assertMessage(expected, actual, event.SubmitStream)
	}

	// stop consumers
	done()
}

func (suite *PrepareTestSuite) TestPrepare_Ads() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL, redisUsername, redisPassword)
	suite.Require().NoError(err)

	// stub payment service with expectedTransactions responses
	server := suite.stubPrepareEndpoint()
	defer server.Close()

	paymentURL := server.URL

	// create and send messages to prepare stream
	messages := make(map[string]event.Message)
	for i := 0; i < 5; i++ {
		transaction := payment.Transaction{
			IdempotencyKey: uuid.NewV4(),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
		}

		body, err := json.Marshal(transaction)
		suite.Require().NoError(err)

		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.Ads,
			Body:      string(body),
		}

		messages[message.ID.String()] = message

		err = redis.Send(context.Background(), message, event.PrepareStream)
		suite.Require().NoError(err)
	}

	_, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	hexPrivateKey := hex.EncodeToString(privateKey)

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, redisUsername)
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, redisPassword)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, hexPrivateKey)
	ctx, done := context.WithTimeout(ctx, 10*time.Second)

	// start prepare consumer
	go prepare.StartConsumer(ctx) // nolint

	// assert message has been processed. once ads messages are consumed by prepare
	// these should be routed to the notify stream
	actualC := make(chan event.Message, len(messages))
	// start a test consumer to read from notify stream
	go test.StartTestBatchConsumer(suite.T(), ctx, redis, event.NotifyStream, actualC)

	for i := 0; i < len(messages); i++ {
		actual := <-actualC
		expected, ok := messages[actual.ID.String()]
		suite.True(ok)
		suite.assertMessage(expected, actual, event.NotifyStream)
	}
	// stop consumers
	done()
}

func (suite *PrepareTestSuite) stubPrepareEndpoint() *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Require().Equal(http.MethodPost, r.Method)
		suite.Require().Equal("/v1/payments/prepare", r.URL.Path)

		w.WriteHeader(http.StatusCreated)

		var transactions []payment.Transaction
		err := json.NewDecoder(r.Body).Decode(&transactions)
		suite.Require().NoError(err)

		for i := 0; i < len(transactions); i++ {
			transactions[i].Custodian = ptr.FromString(transactionstatus.Gemini)
			transactions[i].DocumentID = ptr.FromString(testutils.RandomString())
		}

		payload, err := json.Marshal(transactions)
		suite.Require().NoError(err)

		_, err = w.Write(payload)
	}))

	return ts
}

func (suite *PrepareTestSuite) assertMessage(expected, actual event.Message, stream string) {
	suite.Equal(expected.ID, actual.ID)
	suite.Equal(stream, actual.CurrentStep().Stream)
	suite.NotNil(actual.Routing)

	var expectedTransactions payment.Transaction
	err := json.Unmarshal([]byte(actual.Body), &expectedTransactions)
	suite.Require().NoError(err)

	var actualTransaction payment.Transaction
	err = json.Unmarshal([]byte(actual.Body), &actualTransaction)
	suite.Require().NoError(err)

	suite.Require().Equal(expectedTransactions.From, actualTransaction.From)
	suite.Require().Equal(expectedTransactions.To, actualTransaction.To)
	suite.Require().Equal(expectedTransactions.Amount, actualTransaction.Amount)
	suite.Require().Equal(transactionstatus.Gemini, *actualTransaction.Custodian)
	suite.Require().NotNil(actualTransaction.DocumentID)
}
