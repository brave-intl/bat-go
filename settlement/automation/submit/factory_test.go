//go:build integration

package submit_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/httpsignature"

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

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redis, err := event.NewRedisClient(redisURL, redisUsername, redisPassword)
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
	server := suite.stubSubmitEndpoint(transactions)
	defer server.Close()

	paymentURL := server.URL

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
	go submit.StartConsumer(ctx) // nolint

	// assert message has been processed. once messages are consumed by submit
	// these should be routed to submit status stream
	actualC := make(chan event.Message, len(messages))
	// start a test consumer to read from submit status stream
	go test.StartTestBatchConsumer(suite.T(), ctx, redis, event.SubmitStatusStream, actualC)

	for i := 0; i < len(messages); i++ {
		actual := <-actualC
		expected, ok := messages[actual.ID.String()]
		suite.True(ok)
		suite.assertMessage(expected, actual, event.SubmitStatusStream)
	}
	// stop consumers
	done()
}

func (suite *SubmitTestSuite) stubSubmitEndpoint(transactions map[string]payment.Transaction) *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// assert
		suite.Require().Equal(http.MethodPost, r.Method)
		suite.Require().Equal("/v1/payments/submit", r.URL.Path)
		suite.Require().NotEmpty(r.Header["Digest"])
		suite.Require().NotEmpty(r.Header["Signature"])

		var txns []payment.Transaction
		err := json.NewDecoder(r.Body).Decode(&txns)
		suite.Require().NoError(err)

		for _, actual := range txns {
			expected := transactions[actual.IdempotencyKey.String()]
			suite.assertTransaction(expected, actual)
		}

		// response
		w.WriteHeader(http.StatusCreated)
	}))

	return ts
}

func (suite *SubmitTestSuite) assertMessage(expected, actual event.Message, stream string) {
	suite.Require().Equal(expected.ID, actual.ID)
	suite.Require().Equal(stream, actual.CurrentStep().Stream)
	suite.Require().Equal(expected.Routing.Slip, actual.Routing.Slip)
	suite.Require().Equal(expected.Routing.ErrorHandling, actual.Routing.ErrorHandling)

	var expectedTransactions payment.Transaction
	err := json.Unmarshal([]byte(actual.Body), &expectedTransactions)
	suite.Require().NoError(err)

	var actualTransaction payment.Transaction
	err = json.Unmarshal([]byte(actual.Body), &actualTransaction)
	suite.Require().NoError(err)

	suite.assertTransaction(expectedTransactions, actualTransaction)
}

func (suite *SubmitTestSuite) assertTransaction(expected, actual payment.Transaction) {
	suite.Equal(expected.From, actual.From)
	suite.Equal(expected.To, actual.To)
	suite.Equal(expected.Amount, actual.Amount)
	suite.NotNil(actual.Custodian)
	suite.NotNil(expected.DocumentID)
}
