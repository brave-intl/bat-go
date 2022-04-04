//go:build integration
// +build integration

package prepare_test

import (
	"context"
	"encoding/json"
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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL)
	suite.Require().NoError(err)

	// stub payment service with expectedTransactions responses
	server := stubPrepareEndpoint(suite.T())
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

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx, done := context.WithCancel(ctx)

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
		assertMessage(suite.T(), expected, actual, event.SubmitStream)
	}

	// stop consumers
	done()
}

func (suite *PrepareTestSuite) TestPrepare_Ads() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL)
	suite.Require().NoError(err)

	// stub payment service with expectedTransactions responses
	server := stubPrepareEndpoint(suite.T())
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

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx, done := context.WithCancel(ctx)

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
		assertMessage(suite.T(), expected, actual, event.NotifyStream)
	}
	// stop consumers
	done()
}

func stubPrepareEndpoint(t *testing.T) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/payments/prepare", r.URL.Path)

		w.WriteHeader(http.StatusCreated)

		var transactions []payment.Transaction
		err := json.NewDecoder(r.Body).Decode(&transactions)
		require.NoError(t, err)

		for i := 0; i < len(transactions); i++ {
			transactions[i].Custodian = ptr.FromString(transactionstatus.Gemini)
			transactions[i].DocumentID = ptr.FromString(testutils.RandomString())
		}

		payload, err := json.Marshal(transactions)
		require.NoError(t, err)

		_, err = w.Write(payload)
	}))

	return ts
}

func assertMessage(t *testing.T, expected, actual event.Message, stream string) {
	assert.Equal(t, expected.ID, actual.ID)
	assert.Equal(t, stream, actual.CurrentStep().Stream)
	assert.NotNil(t, actual.Routing)

	var expectedTransactions payment.Transaction
	err := json.Unmarshal([]byte(actual.Body), &expectedTransactions)
	require.NoError(t, err)

	var actualTransaction payment.Transaction
	err = json.Unmarshal([]byte(actual.Body), &actualTransaction)
	require.NoError(t, err)

	assert.Equal(t, expectedTransactions.From, actualTransaction.From)
	assert.Equal(t, expectedTransactions.To, actualTransaction.To)
	assert.Equal(t, expectedTransactions.Amount, actualTransaction.Amount)
	assert.Equal(t, transactionstatus.Gemini, *actualTransaction.Custodian)
	assert.NotNil(t, actualTransaction.DocumentID)
}
