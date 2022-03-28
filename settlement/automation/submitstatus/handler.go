package submitstatus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/settlement/automation/custodian"
	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/backoff/retrypolicy"
	"github.com/brave-intl/bat-go/utils/clients/payment"
	loggingutils "github.com/brave-intl/bat-go/utils/logging"
	"github.com/brave-intl/bat-go/utils/ptr"
)

var (
	retryPolicy        = retrypolicy.DefaultRetry
	nonRetriableErrors = []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden}
)

type submitStatus struct {
	redis       *event.Client
	payment     payment.Client
	retry       backoff.RetryFunc
	checkStatus custodian.StatusResolver
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc,
	checkStatus custodian.StatusResolver) *submitStatus {
	return &submitStatus{
		redis:       redis,
		payment:     payment,
		retry:       retry,
		checkStatus: checkStatus,
	}
}

func (s *submitStatus) Handle(ctx context.Context, messages []event.Message) error {
	var transaction payment.Transaction
	for _, message := range messages {

		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO handle cast failure
			fmt.Println("ERROR UNMARSHAL")
			continue
		}

		if transaction.DocumentID == nil {
			return fmt.Errorf("submit status handler: error documentID is nil for messageID %s", message.ID)
		}

		statusOperation := func() (interface{}, error) {
			return s.payment.Status(ctx, ptr.String(transaction.DocumentID))
		}

		response, err := s.retry(ctx, statusOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			return fmt.Errorf("submit status handler: error calling payment service: %w", err)
		}

		transactionStatus, ok := response.(*payment.TransactionStatus)
		if !ok {
			return fmt.Errorf("submit status handler: error converting transaction status: %w", err)
		}

		status, err := s.checkStatus(*transactionStatus)
		if err != nil {
			return fmt.Errorf("submit status handler: error checking custodian submit status: %w", err)
		}

		switch status {
		case custodian.Complete:
			err := message.Advance()
			if err != nil {
				return fmt.Errorf("submit status handler: error advancing message %s: %w", message.ID, err)
			}

			err = s.redis.Send(ctx, message, message.CurrentStep().Stream)
			if err != nil {
				return fmt.Errorf("submit status handler: error routing message to errored stream %s: %w", message.ID, err)
			}

		case custodian.Pending:
			err := s.redis.Send(ctx, message, message.CurrentStep().Stream)
			if err != nil {
				return fmt.Errorf("submit status handler: error sending message: %w", err)
			}

		case custodian.Failed:
			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("submit status handler: error incrementing error attempt: %w", err)
			}

			err = s.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("submit status handler: error sending message to error stream: %w", err)
			}

		default:
			loggingutils.FromContext(ctx).
				Err(fmt.Errorf("submit status handler: error unknown submit status %s", status)).
				Msg("submit status handler check submit status")

			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("submit status handler: error incrementing error attempt: %w", err)
			}

			err = s.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("submit status handler: error sending message to error stream: %w", err)
			}
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
