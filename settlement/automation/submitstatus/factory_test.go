package submitstatus_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/brave-intl/bat-go/utils/httpsignature"

	"github.com/brave-intl/bat-go/settlement/automation/submitstatus"
	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
	"github.com/brave-intl/bat-go/utils/clients/gemini"
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
			Custodian:      ptr.FromString(transactionstatus.Gemini),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
			DocumentID:     ptr.FromString(documentID),
		}

		body, err := json.Marshal(transaction)
		suite.Require().NoError(err)

		message := event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Type:      event.Ads,
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
		suite.Require().NoError(err)
	}

	// stub payment service with expected response
	server := suite.stubSubmitStatusEndpoint()
	defer server.Close()

	paymentURL := server.URL

	_, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	hexPrivateKey := hex.EncodeToString(privateKey)

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, hexPrivateKey)
	ctx, done := context.WithTimeout(ctx, 10*time.Second)

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
		suite.Require().True(ok)
		suite.assertMessage(expected, actual, event.CheckStatusStream)
	}
	// stop consumers
	done()
}

func (suite *SubmitStatusTestSuite) stubSubmitStatusEndpoint() *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal(http.MethodGet, r.Method)
		// return the transaction for associated documentID with custodian response
		w.WriteHeader(http.StatusOK)

		payoutResult, err := json.Marshal(gemini.PayoutResult{
			Result: "Ok",
			Status: ptr.FromString("completed"),
		})
		suite.Require().NoError(err)

		transactionStatus := payment.TransactionStatus{
			CustodianSubmissionResponse: ptr.FromString(string(payoutResult)),
			Transaction: payment.Transaction{
				IdempotencyKey: uuid.NewV4(),
				Custodian:      ptr.FromString(transactionstatus.Gemini),
				Amount:         decimal.New(1, 0),
				To:             uuid.NewV4(),
				From:           uuid.NewV4(),
				DocumentID:     ptr.FromString(testutils.RandomString()),
			},
		}

		payload, err := json.Marshal(transactionStatus)
		suite.Require().NoError(err)

		_, err = w.Write(payload)
		suite.Require().NoError(err)
	}))

	return ts
}

func (suite *SubmitStatusTestSuite) assertMessage(expected, actual event.Message, stream string) {
	suite.Equal(expected.ID, actual.ID)
	suite.Equal(stream, actual.CurrentStep().Stream)
	suite.Equal(expected.Routing.Slip, actual.Routing.Slip)
	suite.Equal(expected.Routing.ErrorHandling, actual.Routing.ErrorHandling)

	var expectedTransactions payment.Transaction
	err := json.Unmarshal([]byte(actual.Body), &expectedTransactions)
	suite.Require().NoError(err)

	var actualTransaction payment.Transaction
	err = json.Unmarshal([]byte(actual.Body), &actualTransaction)
	suite.Require().NoError(err)

	suite.assertTransaction(expectedTransactions, actualTransaction)
}

func (suite *SubmitStatusTestSuite) assertTransaction(expected, actual payment.Transaction) {
	suite.Equal(expected.From, actual.From)
	suite.Equal(expected.To, actual.To)
	suite.Equal(expected.Amount, actual.Amount)
	suite.NotNil(actual.Custodian)
	suite.NotNil(expected.DocumentID)
}
