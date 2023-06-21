//go:build integration

package internal_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/libs/logging"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/settlementtest"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

type WorkerTestSuite struct {
	suite.Suite
}

func TestWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerTestSuite))
}

func (suite *WorkerTestSuite) SetupTest() {
	settlementtest.StreamsTearDown(suite.T())
}

func (suite *WorkerTestSuite) TestE2ESubmit() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redisAddresses := []string{redisAddress + ":6379"}
	redisClient := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)

	// stub payment service with expected response.
	// this value is set on the message, and we want to assert it is passed to the submit endpoint.
	// Add an arbitrary header value to assert later.
	headerKey, headerValue := testutils.RandomString(), testutils.RandomString()
	server := suite.stubSubmitEndpoint(headerKey, headerValue)
	defer server.Close()

	paymentURL := server.URL

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)

	submitConfig, err := internal.NewSubmitConfig(
		internal.WithRedisAddress(redisAddress),
		internal.WithRedisUsername(redisUsername),
		internal.WithRedisPassword(redisPassword),
		internal.WithPaymentClient(paymentURL))

	// Dynamically created submit stream.
	streamName := testutils.RandomString()
	// Create and send messages to submit stream.
	// Store the txn to assert later.
	transactions := make(map[string]payment.AttestedTransaction)
	for i := 0; i < 10; i++ {
		transaction := payment.AttestedTransaction{
			IdempotencyKey:      uuid.NewV4(),
			Custodian:           testutils.RandomString(),
			To:                  uuid.NewV4(),
			Amount:              decimal.NewFromFloat32(0.0),
			DocumentID:          testutils.RandomString(),
			Version:             testutils.RandomString(),
			State:               testutils.RandomString(),
			AttestationDocument: testutils.RandomString(),
		}

		message, err := event.NewMessage(transaction)
		suite.Require().NoError(err)

		message.SetHeader(headerKey, headerValue)

		err = redisClient.Send(context.Background(), streamName, message)
		suite.Require().NoError(err)

		transactions[transaction.DocumentID] = transaction
	}

	// start submit consumer
	worker := internal.CreateSubmitWorker(submitConfig)
	suite.Require().NoError(err)
	go worker.Run(ctx)

	// Send configuration to the submit config stream.
	config := payout.Config{
		PayoutID:      testutils.RandomString(),
		Stream:        streamName,
		ConsumerGroup: testutils.RandomString(),
		Count:         len(transactions),
	}

	body, err := json.Marshal(config)
	suite.NoError(err)

	message := &event.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Body:      string(body),
	}

	err = redisClient.Send(context.Background(), settlementtest.SubmitConfig, message)
	suite.Require().NoError(err)

	//TODO replace this with payout complete check once implemented
	time.Sleep(20 * time.Second)

	cancel()
}

func (suite *WorkerTestSuite) stubSubmitEndpoint(headerKey, headerValue string) *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// assert
		suite.Require().Equal(http.MethodPost, r.Method)
		suite.Require().Equal("/v1/payments/submit", r.URL.Path)
		suite.Require().Equal(r.Header.Get(headerKey), headerValue)

		var aTxn payment.AttestedTransaction
		err := json.NewDecoder(r.Body).Decode(&aTxn)
		suite.Require().NoError(err)

		// response
		w.WriteHeader(http.StatusOK)
	}))

	return ts
}
