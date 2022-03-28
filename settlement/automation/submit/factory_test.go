//go:build integration
// +build integration

package submit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/logging"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/settlement/automation/submit"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/ptr"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SubmitTestSuite struct {
	suite.Suite
}

func TestSubmitTestSuite(t *testing.T) {
	suite.Run(t, new(SubmitTestSuite))
}

func (suite *SubmitTestSuite) SetupTest() {
	test.StreamsTearDown(suite.T())
}

func (suite *SubmitTestSuite) TestSubmit() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL)
	suite.Require().NoError(err)

	// create and send messages to submit stream
	transactions := make(map[string]payment.Transaction)
	messages := make(map[string]event.Message)
	for i := 0; i < 5; i++ {
		transaction := payment.Transaction{
			IdempotencyKey: uuid.NewV4(),
			Custodian:      ptr.FromString(testutils.RandomString()),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
			DocumentID:     ptr.FromString(testutils.RandomString()),
		}

		transactions[transaction.IdempotencyKey.String()] = transaction

		body, err := json.Marshal(transaction)
		suite.Require().NoError(err)

		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.Grants,
			Routing: &event.Routing{
				Position: 0,
				Slip: []event.Step{
					{
						Stream:  event.SubmitStream,
						OnError: event.ErroredStream,
					},
					{
						Stream:  event.SubmitStatusStream,
						OnError: event.ErroredStream,
					},
				},
				ErrorHandling: event.ErrorHandling{
					MaxRetries: 5,
					Attempt:    0,
				},
			},
			Body: string(body),
		}

		messages[message.ID.String()] = message

		err = redis.Send(context.Background(), message, event.SubmitStream)
		suite.Require().NoError(err)
	}

	// stub payment service with expected response
	server := stubSubmitEndpoint(suite.T(), transactions)
	defer server.Close()

	paymentURL := server.URL

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx, done := context.WithCancel(ctx)

	// start prepare consumer
	go submit.StartConsumer(ctx) // nolint

	// assert message has been processed. once messages are consumed by submit
	// these should be routed to the submit status stream
	actualC := make(chan event.Message, len(messages))
	// start a test consumer to read from submit status stream
	go test.StartTestBatchConsumer(suite.T(), ctx, redis, event.SubmitStatusStream, actualC)

	for i := 0; i < len(messages); i++ {
		actual := <-actualC
		expected, ok := messages[actual.ID.String()]
		suite.True(ok)
		assertMessage(suite.T(), expected, actual, event.SubmitStatusStream)
	}
	// stop consumers
	done()
}

func stubSubmitEndpoint(t *testing.T, transactions map[string]payment.Transaction) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// assert
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/payments/submit", r.URL.Path)

		var txns []payment.Transaction
		err := json.NewDecoder(r.Body).Decode(&txns)
		require.NoError(t, err)

		for _, actual := range txns {
			expected := transactions[actual.IdempotencyKey.String()]
			assertTransaction(t, expected, actual)
		}

		// response
		w.WriteHeader(http.StatusCreated)
	}))

	return ts
}

func assertMessage(t *testing.T, expected, actual event.Message, stream string) {
	assert.Equal(t, expected.ID, actual.ID)
	assert.Equal(t, stream, actual.CurrentStep().Stream)
	assert.Equal(t, expected.Routing.Slip, actual.Routing.Slip)
	assert.Equal(t, expected.Routing.ErrorHandling, actual.Routing.ErrorHandling)

	var expectedTransactions payment.Transaction
	err := json.Unmarshal([]byte(actual.Body), &expectedTransactions)
	require.NoError(t, err)

	var actualTransaction payment.Transaction
	err = json.Unmarshal([]byte(actual.Body), &actualTransaction)
	require.NoError(t, err)

	assertTransaction(t, expectedTransactions, actualTransaction)
}

func assertTransaction(t *testing.T, expected, actual payment.Transaction) {
	assert.Equal(t, expected.From, actual.From)
	assert.Equal(t, expected.To, actual.To)
	assert.Equal(t, expected.Amount, actual.Amount)
	assert.NotNil(t, actual.Custodian)
	assert.NotNil(t, expected.DocumentID)
}
