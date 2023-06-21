//go:build integration

package internal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	awsutils "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/clients/payment"
	"github.com/brave-intl/bat-go/libs/logging"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/event"
	"github.com/brave-intl/bat-go/services/settlement/payout"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal"
	"github.com/brave-intl/bat-go/services/settlement/settlementtest"
	"github.com/go-redis/redis/v8"
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

func (suite *WorkerTestSuite) TestE2EPrepare() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// Create newHandler redis client and clear streams.
	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redisClient, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
	suite.Require().NoError(err)

	// Stub payment service with expectedTransactions responses.
	server := suite.stubPrepareEndpoint()
	defer server.Close()

	paymentURL := server.URL
	bucket := "prepare-attested-transactions"

	// Setup consumer context.
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)

	prepareConfig, err := internal.NewPrepareConfig(
		internal.WithRedisAddress(redisAddress),
		internal.WithRedisUsername(redisUsername),
		internal.WithRedisPassword(redisPassword),
		internal.WithPaymentClient(paymentURL),
		internal.WithReportBucket(bucket),
		internal.WithNotificationTopic(testutils.RandomString()))

	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	logger := logging.Logger(ctx, "prepare-worker-test")

	cfg, err := awsutils.BaseAWSConfig(ctx, logger)
	suite.Require().NoError(err)

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	_, err = s3Client.CreateBucket(context.TODO(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	suite.Require().NoError(err)

	// Dynamically created prepare stream.
	streamName := testutils.RandomString()
	// Create and send messages to prepare stream.
	// Store the txn so we can assert later.
	// For the purpose of the test we are submitting attested transactions.
	var transactions []payment.AttestedTransaction
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

		body, err := json.Marshal(transaction)
		suite.Require().NoError(err)

		message := &event.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Body:      string(body),
		}

		err = redisClient.Send(context.Background(), streamName, message)
		suite.Require().NoError(err)

		// send duplicate message
		err = redisClient.Send(context.Background(), streamName, message)
		suite.Require().NoError(err)

		transactions = append(transactions, transaction)
	}

	// Start prepare worker.
	worker, err := internal.NewPrepareWorker(ctx, prepareConfig)
	suite.Require().NoError(err)
	go worker.Run(ctx)

	// Send configuration to the prepare config stream.
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

	err = redisClient.Send(context.Background(), settlementtest.PrepareConfig, message)
	suite.Require().NoError(err)

	// poll and assert all transactions have been processed.
	var members []redis.Z
	for {
		if len(members) == config.Count {
			break
		}
		members, err = redisClient.ZRangeWithScores(ctx, "prepared-transactions-"+
			config.PayoutID, 0, -1).Result()
		suite.Require().NoError(err)
	}

	// assert the settlement report has been uploaded and contains the attested transactions
	timeout := time.Now().Add(time.Second * 20)
	var out *s3.GetObjectOutput
	for {
		out, err = s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &config.PayoutID,
		})
		if err == nil || time.Now().After(timeout) {
			break
		}
		time.Sleep(time.Millisecond * 50)
	}
	suite.Require().NoError(err)
	defer out.Body.Close()

	var attestedTransactions []payment.AttestedTransaction
	err = json.NewDecoder(out.Body).Decode(&attestedTransactions)
	suite.Require().NoError(err)

	suite.Require().Len(attestedTransactions, len(transactions))
	suite.Assert().ElementsMatch(attestedTransactions, transactions)
}

func (suite *WorkerTestSuite) stubPrepareEndpoint() *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Require().Equal(http.MethodPost, r.Method)
		suite.Require().Equal("/v1/payments/prepare", r.URL.Path)

		w.WriteHeader(http.StatusCreated)

		var aTxn payment.AttestedTransaction
		err := json.NewDecoder(r.Body).Decode(&aTxn)
		suite.Require().NoError(err)

		payload, err := json.Marshal(aTxn)
		suite.Require().NoError(err)

		_, err = w.Write(payload)
	}))

	return ts
}
