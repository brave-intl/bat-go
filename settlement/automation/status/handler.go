package status

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/brave-intl/bat-go/settlement/automation/custodian"

	"net/http"

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

type status struct {
	redis          *event.Client
	payment        payment.Client
	retry          backoff.RetryFunc
	statusResolver custodian.StatusResolver
}

func newHandler(redisClient *event.Client, payment payment.Client, retry backoff.RetryFunc,
	statusResolver custodian.StatusResolver) *status {
	return &status{
		redis:          redisClient,
		payment:        payment,
		retry:          retry,
		statusResolver: statusResolver,
	}
}

func (s *status) Handle(ctx context.Context, messages []event.Message) error {

	loggingutils.FromContext(ctx).Info().Msg("status handler: handling status message")

	var transaction payment.Transaction

	for _, message := range messages {

		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO handle cast failure
			fmt.Println("ERROR UNMARSHAL")
			continue
		}

		if transaction.DocumentID == nil {
			return fmt.Errorf("status handler: error documentID is nil for messageID %s", message.ID)
		}

		statusOperation := func() (interface{}, error) {
			return s.payment.Status(ctx, ptr.String(transaction.DocumentID))
		}

		response, err := s.retry(ctx, statusOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			return fmt.Errorf("status handler: error calling payment service: %w", err)
		}

		transactionStatus, ok := response.(*payment.TransactionStatus)
		if !ok {
			return fmt.Errorf("status handler: error converting transaction status: %w", err)
		}

		status, err := s.statusResolver(*transactionStatus)
		if err != nil {
			return fmt.Errorf("status handler: error checking custodian status: %w", err)
		}

		switch status {
		case custodian.Complete:

			loggingutils.FromContext(ctx).Info().Msg("processing done")

		case custodian.Pending:

			err := s.redis.Send(ctx, message, message.CurrentStep().Stream)
			if err != nil {
				return fmt.Errorf("status handler: error sending message: %w", err)
			}

		case custodian.Failed:

			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("status handler: error incrementing error attempt: %w", err)
			}

			err = s.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("status handler: error sending message to error stream: %w", err)
			}

		default:

			loggingutils.FromContext(ctx).
				Err(fmt.Errorf("status handler: error unknown status %s", status)).
				Msg("status handler check status")

			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("status handler: error incrementing error attempt: %w", err)
			}

			err = s.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("status handler: error sending message to error stream: %w", err)
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
