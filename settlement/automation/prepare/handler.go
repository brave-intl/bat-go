package prepare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	loggingutils "github.com/brave-intl/bat-go/utils/logging"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	uuid "github.com/satori/go.uuid"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type prepare struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc) *prepare {
	return &prepare{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

func (p *prepare) Handle(ctx context.Context, messages []event.Message) error {

	loggingutils.FromContext(ctx).Info().Msg("prepare handler: handling prepare message")

	// map and batch the transactions

	transactionToMessage := make(map[uuid.UUID]event.Message)

	var transaction payment.Transaction
	var transactions []payment.Transaction

	for _, message := range messages {
		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO dlq
			fmt.Println("ERROR UNMARSHAL")
			continue
		}
		transactions = append(transactions, transaction)
		transactionToMessage[transaction.IdempotencyKey] = message
	}

	// check we have transactions to process
	if len(transactions) < 1 {
		return nil
	}

	// wrap prepare call in retriable operation
	prepareOperation := func() (interface{}, error) {
		return p.payment.Prepare(ctx, transactions)
	}

	response, err := p.retry(ctx, prepareOperation, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return fmt.Errorf("prepare handler: error calling payment service: %w", err)
	}

	// update messages with response transactions and send to next destination

	txns, ok := response.(*[]payment.Transaction)
	if !ok {
		return fmt.Errorf("prepare handler: error converting response transactions: %w", err)
	}

	for _, txn := range *txns {
		// retrieve original message and update with response transaction
		message, ok := transactionToMessage[txn.IdempotencyKey]
		if !ok {
			return fmt.Errorf("prepare handler: error could not retrieve original message %s: %w", message.ID, err)
		}

		err = message.SetBody(txn)
		if err != nil {
			return fmt.Errorf("prepare handler: error could not set body for messageID %s: %w", message.ID, err)
		}

		// send message to destination
		switch {
		case p.isFailed(txn):
			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("prepare handler: error incrementing message error attempt %s: %w", message.ID, err)
			}

			err = p.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("prepare handler: error routing message to errored stream %s: %w", message.ID, err)
			}
		default:
			err := message.Advance()
			if err != nil {
				return fmt.Errorf("prepare handler: error advancing message %s: %w", message.ID, err)
			}

			err = p.redis.Send(ctx, message, message.CurrentStep().Stream)
			if err != nil {
				return fmt.Errorf("prepare handler: error routing message to errored stream %s: %w", message.ID, err)
			}
		}
	}

	return nil
}

// isFailed checks if a transaction was successfully accepted by payment service prepare
func (p *prepare) isFailed(transaction payment.Transaction) bool {
	return transaction.DocumentID == nil
}

func canRetry(nonRetriableErrors []int) func(error) bool {
	return func(err error) bool {
		if paymentError, err := payment.UnwrapPaymentError(err); err == nil {
			for _, httpStatusCode := range nonRetriableErrors {
				if paymentError.Code == httpStatusCode {
					return false
				}
			}
		}
		return true
	}
}
