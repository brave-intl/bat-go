//go:build integration
// +build integration

package status_test

import (
	"context"
	"encoding/json"
	"github.com/brave-intl/bat-go/settlement/automation/status"
	"github.com/brave-intl/bat-go/settlement/automation/test"
	"github.com/brave-intl/bat-go/utils/logging"
	"strings"

	"github.com/brave-intl/bat-go/utils/ptr"

	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	appctx "github.com/brave-intl/bat-go/utils/context"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type StatusTestSuite struct {
	suite.Suite
}

func TestStatusTestSuite(t *testing.T) {
	suite.Run(t, new(StatusTestSuite))
}

func (suite *StatusTestSuite) SetupTest() {
	test.StreamsTearDown(suite.T())
}

func (suite *StatusTestSuite) TestStatus() {
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
			Body:      string(body),
		}

		messages[documentID] = message

		err = redis.Send(context.Background(), message, event.CheckStatusStream)
		suite.NoError(err)
	}

	// stub payment service with expected response
	server := stubStatusEndpoint(suite.T(), messages)
	defer server.Close()

	paymentURL := server.URL

	// setup consumer context
	ctx := context.Background()
	ctx, _ = logging.SetupLogger(ctx)
	ctx = context.WithValue(ctx, appctx.RedisSettlementURLCTXKey, redisURL)
	ctx = context.WithValue(ctx, appctx.PaymentServiceURLCTXKey, paymentURL)
	ctx, done := context.WithCancel(ctx)

	// start prepare consumer
	go status.StartConsumer(ctx) // nolint

	// assert all messaged have been processed
	for {
		xLen, err := redis.XLen(ctx, event.CheckStatusStream).Result()
		suite.NoError(err)
		xPending, _ := redis.XPending(ctx, event.CheckStatusStream,
			event.CheckStatusConsumerGroup).Result()
		if xPending != nil && xLen == 5 && xPending.Count == 0 {
			break
		}
	}
	// stop consumers
	done()
}

func stubStatusEndpoint(t *testing.T, messages map[string]event.Message) *httptest.Server {
	t.Helper()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)

		// assert we received the documentID
		documentID := strings.Split(r.URL.Path, "/")[3]
		_, ok := messages[documentID]
		assert.True(t, ok)

		// return the transaction for associated documentID with custodian response
		w.WriteHeader(http.StatusOK)

		transactionStatus := payment.TransactionStatus{
			CustodianStatusResponse: testutils.RandomString(),
			Transaction: payment.Transaction{
				IdempotencyKey: uuid.NewV4(),
				Custodian:      ptr.FromString(testutils.RandomString()),
				Amount:         decimal.New(1, 0),
				To:             uuid.NewV4(),
				From:           uuid.NewV4(),
				DocumentID:     ptr.FromString(documentID),
			},
		}

		payload, err := json.Marshal(transactionStatus)
		assert.NoError(t, err)

		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))

	return ts
}
