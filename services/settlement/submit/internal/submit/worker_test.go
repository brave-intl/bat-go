//go:build integration

package submit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
	"github.com/brave-intl/bat-go/services/settlement/submit/internal/submit"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
)

// TODO(clD11) rewrite tests using new approach

type WorkerTestSuite struct {
	suite.Suite
}

func TestWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerTestSuite))
}

func (suite *WorkerTestSuite) TestE2ESubmit() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redisClient := redis.NewClient(redisAddress+":6379", redisUsername, redisPassword)

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

	submitConfigStream := testutils.RandomString()
	workerConfig, err := submit.NewWorkerConfig(
		submit.WithRedisAddress(redisAddress),
		submit.WithRedisUsername(redisUsername),
		submit.WithRedisPassword(redisPassword),
		submit.WithPaymentClient(paymentURL),
		submit.WithConfigStream(submitConfigStream))

	// Dynamically created submit stream.
	streamName := testutils.RandomString()
	// Create and send messages to submit stream.
	// Store the txn to assert later.
	attestedDetails := make(map[string]payment.AttestedDetails)
	for i := 0; i < 10; i++ {
		ad := payment.AttestedDetails{
			Details: payment.Details{
				To:        testutils.RandomString(),
				From:      testutils.RandomString(),
				Amount:    decimal.New(10, 0),
				Custodian: testutils.RandomString(),
				PayoutID:  testutils.RandomString(),
				Currency:  testutils.RandomString(),
			},
			DocumentID:          testutils.RandomString(),
			AttestationDocument: testutils.RandomString(),
		}

		message, err := consumer.NewMessage(ad)
		suite.Require().NoError(err)

		message.SetHeader(headerKey, headerValue)

		args := redis.XAddArgs{
			Stream: streamName,
			Values: map[string]interface{}{"data": message},
		}

		err = redisClient.XAdd(context.Background(), args)
		suite.Require().NoError(err)

		attestedDetails[ad.DocumentID] = ad
	}

	// start submit consumer
	worker, err := submit.CreateWorker(workerConfig)
	suite.Require().NoError(err)

	go worker.Run(ctx)

	// Send configuration to the submit config stream.
	config := payment.Config{
		PayoutID:      testutils.RandomString(),
		Stream:        streamName,
		ConsumerGroup: testutils.RandomString(),
		Count:         len(attestedDetails),
	}

	body, err := json.Marshal(config)
	suite.NoError(err)

	message := &consumer.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Body:      string(body),
	}

	args := redis.XAddArgs{
		Stream: submitConfigStream,
		Values: map[string]interface{}{"data": message},
	}

	err = redisClient.XAdd(context.Background(), args)
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

		var aTxn payment.AttestedDetails
		err := json.NewDecoder(r.Body).Decode(&aTxn)
		suite.Require().NoError(err)

		// response
		w.WriteHeader(http.StatusOK)
	}))

	return ts
}
