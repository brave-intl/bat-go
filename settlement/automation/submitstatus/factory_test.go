package submitstatus_test

import (
	"context"
	"encoding/json"

	"github.com/brave-intl/bat-go/settlement/automation/submitstatus"
	"github.com/brave-intl/bat-go/utils/logging"

	"github.com/brave-intl/bat-go/settlement/automation/event"

	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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

type SubmitStatusTestSuite struct {
	suite.Suite
}

func TestSubmitStatusTestSuite(t *testing.T) {
	suite.Run(t, new(SubmitStatusTestSuite))
}

func (suite *SubmitStatusTestSuite) SetupTest() {
	test.StreamsTearDown(suite.T())
}

func (suite *SubmitStatusTestSuite) TestSubmitStatus() {
	redisURL := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisURL)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL)
	suite.Require().NoError(err)

	// create and send messages to check status stream
	messages := make(map[string]event.Message)
	for i := 0; i < 5; i++ {
		documentID := testutils.RandomString()
		transaction := payment.Transaction{
			IdempotencyKey: uuid.NewV4(),
			Custodian:      ptr.FromString(testutils.RandomString()),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
			DocumentID:     ptr.FromString(documentID),
		}

		body, err := json.Marshal(transaction)
		suite.NoError(err)

		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.Grants,
			Routing: &event.Routing{
				Position: 0,
				Slip: []event.Step{
					{
						Stream:  event.SubmitStatusStream,
						OnError: event.ErroredStream,
					},
					{
						Stream:  event.CheckStatusStream,
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

		err = redis.Send(context.Background(), message, event.SubmitStatusStream)
		suite.NoError(err)
	}

	// stub payment service with expected response
	server := stubSubmitStatusEndpoint(suite.T(), messages)
	defer server.Close()

	paymentURL := server.URL

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx, done := context.WithCancel(ctx)

	// start prepare consumer
	go submitstatus.StartConsumer(ctx) // nolint

	// assert message has been processed. once messages are consumed by submit status
	// these should be routed to the check status stream
	actualC := make(chan event.Message, len(messages))
	// start a test consumer to read from check status stream
	go test.StartTestBatchConsumer(suite.T(), ctx, redis, event.CheckStatusStream, actualC)

	for i := 0; i < len(messages); i++ {
		actual := <-actualC
		expected, ok := messages[actual.ID.String()]
		suite.True(ok)
		assertMessage(suite.T(), expected, actual, event.CheckStatusStream)
	}
	// stop consumers
	done()
}

func stubSubmitStatusEndpoint(t *testing.T, messages map[string]event.Message) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		// return the transaction for associated documentID with custodian response
		w.WriteHeader(http.StatusOK)

		transactionStatus := payment.TransactionStatus{
			CustodianSubmissionResponse: testutils.RandomString(),
			Transaction: payment.Transaction{
				IdempotencyKey: uuid.NewV4(),
				Custodian:      ptr.FromString(testutils.RandomString()),
				Amount:         decimal.New(1, 0),
				To:             uuid.NewV4(),
				From:           uuid.NewV4(),
				DocumentID:     ptr.FromString(testutils.RandomString()),
			},
		}

		payload, err := json.Marshal(transactionStatus)
		assert.NoError(t, err)

		_, err = w.Write(payload)
		assert.NoError(t, err)
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
