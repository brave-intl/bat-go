package submit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients/payment"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type submit struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc) *submit {
	return &submit{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

func (s *submit) Handle(ctx context.Context, messages []event.Message) error {

	// batch the transactions
	var transaction payment.Transaction
	var transactions []payment.Transaction

	for _, message := range messages {
		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO handle cast failure
			fmt.Println("ERROR UNMARSHAL")
			continue
		}
		transactions = append(transactions, transaction)
	}

	// check we have transactions to process
	if len(transactions) < 1 {
		return nil
	}

	// submit transactions to payment service with retry
	submitOperation := func() (interface{}, error) {
		return nil, s.payment.Submit(ctx, transactions)
	}

	_, err := s.retry(ctx, submitOperation, retryPolicy, canRetry(nonRetriableErrors))
	if err != nil {
		return fmt.Errorf("submit handler: error calling payment service: %w", err)
	}

	// send each message to next destination
	for _, message := range messages {
		err := message.Advance()
		if err != nil {
			return fmt.Errorf("submit handler: error advancing message %s: %w", message.ID, err)
		}

		err = s.redis.Send(ctx, message, message.CurrentStep().Stream)
		if err != nil {
			return fmt.Errorf("submit handler: error routing message to errored stream %s: %w", message.ID, err)
		}
	}

	return nil
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
