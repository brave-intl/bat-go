//go:build integration

package checkstatus_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/brave-intl/bat-go/settlement/automation/checkstatus"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/logging"
	testutils "github.com/brave-intl/bat-go/utils/test"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
	"github.com/brave-intl/bat-go/utils/clients/gemini"

	"github.com/brave-intl/bat-go/utils/ptr"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type CheckStatusTestSuite struct {
	suite.Suite
}

func TestCheckStatusTestSuite(t *testing.T) {
	suite.Run(t, new(CheckStatusTestSuite))
}

func (suite *CheckStatusTestSuite) SetupTest() {
	test.StreamsTearDown(suite.T())
}

func (suite *CheckStatusTestSuite) TestCheckStatus() {
	redisAddress := os.Getenv("REDIS_URL")
	suite.Require().NotNil(redisAddress)

	redisUsername := os.Getenv("REDIS_USERNAME")
	suite.Require().NotNil(redisUsername)

	redisPassword := os.Getenv("REDIS_PASSWORD")
	suite.Require().NotNil(redisPassword)

	// create newHandler redis client and clear streams
	redisAddresses := []string{fmt.Sprintf("%s:6379", redisAddress)}
	redis, err := event.NewRedisClient(redisAddresses, redisUsername, redisPassword)
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
			Type:      event.Grants,
			Routing: &event.Routing{
				Slip: []event.Step{
					{
						Stream:  event.CheckStatusStream,
						OnError: event.ErroredStream,
					},
				},
			},
			Body: string(body),
		}

		messages[documentID] = message

		err = redis.Send(context.Background(), message, event.CheckStatusStream)
		suite.Require().NoError(err)
	}

	// stub payment service with expected response
	server := suite.stubStatusEndpoint(messages)
	defer server.Close()

	paymentURL := server.URL

	_, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	suite.Require().NoError(err)

	hexPrivateKey := hex.EncodeToString(privateKey)

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.SettlementRedisAddressCTXKey, redisAddress)
	ctx = context.WithValue(ctx, appctx.SettlementRedisUsernameCTXKey, redisUsername)
	ctx = context.WithValue(ctx, appctx.SettlementRedisPasswordCTXKey, redisPassword)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceHTTPSingingKeyHexCTXKey, hexPrivateKey)

	ctx, done := context.WithTimeout(ctx, 10*time.Second)

	s, err := redis.XGroupCreateMkStream(ctx, event.CheckStatusStream, event.CheckStatusConsumerGroup, "0").Result()
	fmt.Println(s)
	fmt.Println(err)

	// start prepare consumer
	go checkstatus.StartConsumer(ctx) // nolint

	timer := time.Now().Add(10 * time.Second)
	for {
		// keep checking until all messages have been acknowledged before asserting
		xPending, _ := redis.XPending(ctx, event.CheckStatusStream, event.CheckStatusConsumerGroup).Result()

		// check all messages have been ack before asserting
		if xPending != nil && xPending.Count == int64(0) {
			// assert all messages were successfully written to stream
			streamCount, err := redis.XLen(ctx, event.CheckStatusStream).Result()
			suite.NoError(err)
			suite.Require().Equal(int64(len(messages)), streamCount)

			// assert the dlq is empty
			DLQCount, err := redis.XLen(ctx, event.DeadLetterQueue).Result()
			suite.NoError(err)
			suite.Equal(int64(0), DLQCount)

			break
		}
		if time.Now().After(timer) {
			suite.Fail("test timeout")
			break
		}
	}
	done()
}

func (suite *CheckStatusTestSuite) stubStatusEndpoint(messages map[string]event.Message) *httptest.Server {
	suite.T().Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suite.Equal(http.MethodGet, r.Method)

		// assert we received the documentID
		documentID := strings.Split(r.URL.Path, "/")[3]
		_, ok := messages[documentID]
		suite.True(ok)

		// return the transaction for associated documentID with custodian response
		w.WriteHeader(http.StatusOK)

		payoutResult := gemini.PayoutResult{
			Result: "Ok",
			Status: ptr.FromString("completed"),
		}

		pr, err := json.Marshal(payoutResult)

		transactionStatus := payment.TransactionStatus{
			CustodianStatusResponse: ptr.FromString(string(pr)),
			Transaction: payment.Transaction{
				IdempotencyKey: uuid.NewV4(),
				Custodian:      ptr.FromString(transactionstatus.Gemini),
				Amount:         decimal.New(1, 0),
				To:             uuid.NewV4(),
				From:           uuid.NewV4(),
				DocumentID:     ptr.FromString(documentID),
			},
		}

		payload, err := json.Marshal(transactionStatus)
		suite.NoError(err)

		_, err = w.Write(payload)
		suite.NoError(err)
	}))

	return ts
}
