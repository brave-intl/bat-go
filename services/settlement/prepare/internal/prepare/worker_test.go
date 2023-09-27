//go:build integration

package prepare_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/shopspring/decimal"

	awsutils "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	testutils "github.com/brave-intl/bat-go/libs/test"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer"
	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
	"github.com/brave-intl/bat-go/services/settlement/internal/payment"
	"github.com/brave-intl/bat-go/services/settlement/prepare/internal/prepare"

	uuid "github.com/satori/go.uuid"

	"github.com/stretchr/testify/suite"
)

type WorkerTestSuite struct {
	suite.Suite
}

func TestWorkerTestSuite(t *testing.T) {
	suite.Run(t, new(WorkerTestSuite))
}

func (suite *WorkerTestSuite) TestE2EPrepare() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// Create newHandler redis client and clear streams.
	redisClient := redis.NewClient(redisAddress+":6379", redisUsername, redisPassword)

	// Stub payment service with expectedTransactions responses.
	server := suite.stubPrepareEndpoint()
	defer server.Close()

	paymentURL := server.URL
	bucket := "prepare-attested-transactions"

	// Setup consumer context.
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)

	payoutConfigStream := testutils.RandomString()
	workerConfig, err := prepare.NewWorkerConfig(
		prepare.WithRedisAddress(redisAddress),
		prepare.WithRedisUsername(redisUsername),
		prepare.WithRedisPassword(redisPassword),
		prepare.WithPaymentClient(paymentURL),
		prepare.WithConfigStream(payoutConfigStream),
		prepare.WithReportBucket(bucket),
		prepare.WithNotificationTopic(testutils.RandomString()))

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
	// We are also sending duplicate messages so a total of 20 messages will be sent.
	var transactions []payment.AttestedDetails
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

		body, err := json.Marshal(ad)
		suite.Require().NoError(err)

		message := &consumer.Message{
			ID:        uuid.NewV4(),
			Timestamp: time.Now(),
			Body:      string(body),
		}

		args := redis.XAddArgs{
			Stream: streamName,
			Values: map[string]interface{}{"data": message},
		}

		err = redisClient.XAdd(context.Background(), args)
		suite.Require().NoError(err)

		// send duplicate message
		err = redisClient.XAdd(context.Background(), args)
		suite.Require().NoError(err)

		transactions = append(transactions, ad)
	}

	// Start prepare worker.
	worker, err := prepare.CreateWorker(ctx, workerConfig)
	suite.Require().NoError(err)

	go worker.Run(ctx)

	// Send the payout configuration to the prepare config stream.
	payoutConfig := payment.Config{
		PayoutID:      testutils.RandomString(),
		Stream:        streamName,
		ConsumerGroup: testutils.RandomString(),
		Count:         len(transactions),
	}

	body, err := json.Marshal(payoutConfig)
	suite.NoError(err)

	message := &consumer.Message{
		ID:        uuid.NewV4(),
		Timestamp: time.Now(),
		Body:      string(body),
	}

	args := redis.XAddArgs{
		Stream: payoutConfigStream,
		Values: map[string]interface{}{"data": message},
	}

	err = redisClient.XAdd(context.Background(), args)
	suite.Require().NoError(err)

	// poll until all transactions have been processed.
	for {
		members, err := redisClient.ZRange(ctx, "txn-store-"+payoutConfig.PayoutID, 0, -1)
		suite.Require().NoError(err)

		if len(members) == payoutConfig.Count {
			break
		}
	}

	// Assert the settlement report has been uploaded and contains the attested transactions. The worker should have
	// filtered out all duplicate messages during processing.
	timeout := time.Now().Add(time.Second * 20)
	var out *s3.GetObjectOutput
	for {
		out, err = s3Client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    &payoutConfig.PayoutID,
		})
		if err == nil || time.Now().After(timeout) {
			break
		}
		time.Sleep(time.Millisecond * 50)
	}
	suite.Require().NoError(err)
	defer out.Body.Close()

	var attestedDetails []payment.AttestedDetails
	err = json.NewDecoder(out.Body).Decode(&attestedDetails)
	suite.Require().NoError(err)

	suite.Require().Len(attestedDetails, len(transactions))
	suite.Assert().ElementsMatch(attestedDetails, transactions)
}

func (suite *WorkerTestSuite) stubPrepareEndpoint() *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Require().Equal(http.MethodPost, r.Method)
		suite.Require().Equal("/v1/payments/prepare", r.URL.Path)

		var aTxn payment.AttestedDetails
		err := json.NewDecoder(r.Body).Decode(&aTxn)
		suite.Require().NoError(err)

		payload, err := json.Marshal(aTxn)
		suite.Require().NoError(err)

		w.Header().Set("x-nitro-attestation", aTxn.AttestationDocument)
		w.WriteHeader(http.StatusCreated)

		_, err = w.Write(payload)
	}))

	return ts
}
