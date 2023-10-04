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
	. "github.com/brave-intl/bat-go/libs/payments"
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
func (zm *ZebpayMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return zm.transaction, err
	}

	// Do nothing if the state is already final
	if zm.transaction.Status == Paid || zm.transaction.Status == Failed {
		return zm.transaction, nil
	}

	var (
		entry *AuthenticatedPaymentState
	)

	if zm.transaction.Status == Pending {
		// We don't want to check status too fast
		time.Sleep(zm.backoffFactor * time.Millisecond)
		// Get status of transaction and update the state accordingly
		ctr, err := zm.client.CheckTransfer(ctx, &zebpay.ClientOpts{
			APIKey: zm.apiKey, SigningKey: zm.signingKey,
		}, zm.transaction.IdempotencyKey())
		if err != nil {
			return nil, fmt.Errorf("failed to check transaction status: %w", err)
		}
		switch ctr.Code {
		case zebpay.TransferSuccessCode:
			// Write the Paid status and end the loop by not calling Drive
			entry, err = zm.SetNextState(ctx, Paid)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
		case zebpay.TransferPendingCode:
			// Set backoff without changing status
			zm.backoffFactor = zm.backoffFactor * 2
			entry, err = Drive(ctx, zm)
			if err != nil {
				return entry, fmt.Errorf(
					"failed to drive transaction from pending to paid: %w",
					err,
				)
			}
		default:
			// Status unknown. includes TransferFailedCode
			entry, err = zm.SetNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from zebpay for transaction: %v",
				entry,
			)
		}
	} else {
		to, err := strconv.ParseInt(zm.transaction.To, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("transaction to is not well formed: %w", err)
		}
		// submit the transaction
		btr, err := zm.client.BulkTransfer(
			ctx,
			&zebpay.ClientOpts{
				APIKey: zm.apiKey,
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
			return nil, fmt.Errorf("failed to check transaction status: %w", err)
		}

		if strings.ToUpper(btr.Data) != "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED" {
			// Status unknown. Fail
			entry, err = zm.SetNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from zebpay for transaction: %v",
				entry,
			)
		}

		entry, err = zm.SetNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}

		// Set initial backoff
		zm.backoffFactor = 2
		entry, err = Drive(ctx, zm)
		if err != nil {
			return entry, fmt.Errorf(
				"failed to drive transaction from pending to paid: %w",
				err,
			)
		}
	}
	return zm.transaction, nil
}

// Fail implements TxStateMachine for the Zebpay machine.
func (bm *ZebpayMachine) Fail(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return bm.SetNextState(ctx, Failed)
}
