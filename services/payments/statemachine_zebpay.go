package payments

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/zebpay"
	paymentLib "github.com/brave-intl/bat-go/libs/payments"
	"github.com/google/uuid"
)

// ZebpayMachine is an implementation of TxStateMachine for Zebpay's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type ZebpayMachine struct {
	baseStateMachine
	client        zebpay.Client
	apiKey        string
	signingKey    crypto.PrivateKey
	zebpayHost    string
	backoffFactor time.Duration
}

// Pay implements TxStateMachine for the Zebpay machine.
func (zm *ZebpayMachine) Pay(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return zm.transaction, err
	}

	// Do nothing if the state is already final
	if zm.transaction.Status == paymentLib.Paid || zm.transaction.Status == paymentLib.Failed {
		return zm.transaction, nil
	}

	var (
		entry *paymentLib.AuthenticatedPaymentState
	)

	to, err := strconv.ParseInt(zm.transaction.To, 10, 64)
	if err != nil {
		return zm.transaction, fmt.Errorf("transaction to is not well formed: %w", err)
	}

	if zm.transaction.Status == paymentLib.Pending {
		// We don't want to check status too fast
		time.Sleep(zm.backoffFactor * time.Millisecond)
		// Get status of transaction and update the state accordingly
		ctr, cterr := zm.client.CheckTransfer(ctx, &zebpay.ClientOpts{
			APIKey: zm.apiKey, SigningKey: zm.signingKey,
		}, zm.transaction.IdempotencyKey())

		switch ctr.Code {
		case zebpay.TransferSuccessCode:
			// Write the Paid status
			// entry is unused after this
			_, err = zm.SetNextState(ctx, paymentLib.Paid)
			if err != nil {
				return zm.transaction, fmt.Errorf("failed to write next state: %w", err)
			}
		case zebpay.TransferPendingCode:
			// Set backoff without changing status
			zm.backoffFactor = zm.backoffFactor * 2
		case zebpay.TransferNotFoundCode:
			// The transaction was never received by zebpay. We should not have gotten into this
			// condition unless zebpay accepted this transaction, but failed to persist the record.
			// We'll need to retry the submission and come back around to check status, so do not
			// SetNextState except those that get set as part of the submit function
			return zm.transaction, zm.submit(ctx, entry, to)
		default:
			if cterr != nil {
				return zm.transaction, fmt.Errorf("failed to check transaction status: %w", err)
			}
			// Status unknown. includes TransferFailedCode
			// entry is unused after this
			entry, err = zm.SetNextState(ctx, paymentLib.Failed)
			if err != nil {
				return zm.transaction, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from zebpay for transaction: %v",
				entry,
			)
		}
	} else {
		// Get status of transaction and update the state accordingly. The only acceptable error at
		// this point is 404, indicating that this transaction has not yet been received by zebpay.
		ctr, err := zm.client.CheckTransfer(ctx, &zebpay.ClientOpts{
			APIKey: zm.apiKey, SigningKey: zm.signingKey,
		}, zm.transaction.IdempotencyKey())

		switch ctr.Code {
		case zebpay.TransferSuccessCode:
			// Write the Paid status
			// entry is unused after this
			_, err = zm.SetNextState(ctx, paymentLib.Paid)
			if err != nil {
				return zm.transaction, fmt.Errorf("failed to write next state: %w", err)
			}
			return zm.transaction, nil
		case zebpay.TransferPendingCode:
			// Transfer is already Pending, but isn't recorded as such in QLDB. Set it to
			// Pending in QLDB and proceed.
			// entry is unused after this
			_, err = zm.SetNextState(ctx, paymentLib.Pending)
			if err != nil {
				return zm.transaction, fmt.Errorf("failed to write next state: %w", err)
			}
			// Set backoff without changing status
			zm.backoffFactor = zm.backoffFactor * 2
		case zebpay.TransferNotFoundCode:
			// Continue to submission without state change if zebpay is unaware of the
			// transaction
			break
		default:
			if err != nil {
				return zm.transaction, fmt.Errorf("transfer check failed with error: %w", err)
			}
			return zm.transaction, errors.New("unknown zebpay response")
		}

		err = zm.submit(ctx, entry, to)
		if err != nil {
			return zm.transaction, err
		}

		// entry is unused after this
		_, err = zm.SetNextState(ctx, paymentLib.Pending)
		if err != nil {
			return zm.transaction, fmt.Errorf("failed to write next state: %w", err)
		}

		// Set initial backoff
		zm.backoffFactor = 2
	}

	return zm.transaction, nil
}

// Fail implements TxStateMachine for the Zebpay machine.
func (zm *ZebpayMachine) Fail(ctx context.Context) (*paymentLib.AuthenticatedPaymentState, error) {
	return zm.SetNextState(ctx, paymentLib.Failed)
}

func (zm *ZebpayMachine) submit(
	ctx context.Context,
	entry *paymentLib.AuthenticatedPaymentState,
	to int64,
) error {
	// submit the transaction
	btr, err := zm.client.BulkTransfer(
		ctx,
		&zebpay.ClientOpts{
			APIKey:     zm.apiKey,
			SigningKey: zm.signingKey,
		},
		zebpay.NewBulkTransferRequest(
			zebpay.NewTransfer(
				zm.transaction.IdempotencyKey(),
				uuid.MustParse(zm.transaction.From),
				zm.transaction.Amount,
				to,
			),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to submit zebpay transaction: %w", err)
	}
	if strings.ToUpper(btr.Data) != "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED" {
		// Status unknown. Fail
		entry, err = zm.SetNextState(ctx, paymentLib.Failed)
		if err != nil {
			return fmt.Errorf("failed to write next state: %w", err)
		}
		return fmt.Errorf(
			"received unknown status from zebpay for transaction: %v",
			entry,
		)
	}
	return err
}
