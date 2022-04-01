package checkstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/settlement/automation/transactionstatus"
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

type checkStatus struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
	resolve transactionstatus.Resolver
}

func newHandler(redisClient *event.Client, payment payment.Client, retry backoff.RetryFunc,
	resolver transactionstatus.Resolver) *checkStatus {
	return &checkStatus{
		redis:   redisClient,
		payment: payment,
		retry:   retry,
		resolve: resolver,
	}
}

func (c *checkStatus) Handle(ctx context.Context, messages []event.Message) error {
	loggingutils.FromContext(ctx).Info().Msg("check status handler: handling message")

	var transaction payment.Transaction
	for _, message := range messages {

		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO handle cast failure
			fmt.Println("ERROR UNMARSHAL")
			continue
		}

		if transaction.DocumentID == nil {
			return fmt.Errorf("check status handler: error documentID is nil for messageID %s", message.ID)
		}

		statusOperation := func() (interface{}, error) {
			return c.payment.Status(ctx, ptr.String(transaction.DocumentID))
		}

		response, err := c.retry(ctx, statusOperation, retryPolicy, canRetry(nonRetriableErrors))
		if err != nil {
			return fmt.Errorf("check status handler: error calling payment service: %w", err)
		}

		transactionStatus, ok := response.(*payment.TransactionStatus)
		if !ok {
			return fmt.Errorf("check status handler: error converting transaction status: %w", err)
		}

		status, err := c.resolve(*transactionStatus)
		if err != nil {
			return fmt.Errorf("check status handler: error checking custodian status: %w", err)
		}

		switch status {
		case transactionstatus.Complete:

			loggingutils.FromContext(ctx).Info().
				Msgf("check status handler: transaction with documentID %s complete",
					*transactionStatus.Transaction.DocumentID)

		case transactionstatus.Pending:

			err := c.redis.Send(ctx, message, message.CurrentStep().Stream)
			if err != nil {
				return fmt.Errorf("check status handler: error sending message: %w", err)
			}

		case transactionstatus.Failed:

			err := message.IncrementErrorAttempt()
			if err != nil {
				return fmt.Errorf("check status handler: error incrementing error attempt: %w", err)
			}

			err = c.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("check status handler: error sending message to error stream: %w", err)
			}

		// for an unknown transaction status we can retry until we reach max attempts before sending
		// to the error stream.
		case transactionstatus.Unknown:

			err := message.IncrementErrorAttempt()
			switch {
			case errors.Is(err, event.ErrMaxRetriesExceeded):

				loggingutils.FromContext(ctx).Warn().
					Msgf("max check status attempts have been reached for messageID %s", message.ID)

				err := c.redis.Send(ctx, message, message.CurrentStep().OnError)
				if err != nil {
					return fmt.Errorf("error sending message: %w", err)
				}

			default:
				err := c.redis.Send(ctx, message, message.CurrentStep().Stream)
				if err != nil {
					return fmt.Errorf("error sending message: %w", err)
				}
			}

		default:
			loggingutils.FromContext(ctx).
				Err(fmt.Errorf("check status handler: error invalid status %s", status)).
				Msg("check status handler")

			err = c.redis.Send(ctx, message, message.CurrentStep().OnError)
			if err != nil {
				return fmt.Errorf("check status handler: error sending message to error stream: %w", err)
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
