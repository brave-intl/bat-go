package payments

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/zebpay"
	. "github.com/brave-intl/bat-go/libs/payments"
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
func (bm *ZebpayMachine) Pay(ctx context.Context) (*AuthenticatedPaymentState, error) {
	err := ctx.Err()
	if errors.Is(err, context.DeadlineExceeded) {
		return bm.transaction, err
	}

	// Do nothing if the state is already final
	if bm.transaction.Status == Paid || bm.transaction.Status == Failed {
		return bm.transaction, nil
	}

	if bm.transaction.Status == Pending {
		// We don't want to check status too fast
		time.Sleep(bm.backoffFactor * time.Millisecond)
		// Get status of transaction and update the state accordingly
		ctr, err := bm.client.CheckTransfer(ctx, zebpay.ClientOpts{
			APIKey: bm.apiKey, SigningKey: bm.signingKey,
		}, bm.transaction.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check transaction status: %w", err)
		}
		switch ctr.Code {
		case zebpay.TransferSuccessCode:
			// Write the Paid status and end the loop by not calling Drive
			entry, err = bm.writeNextState(ctx, Paid)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
		case zebpay.TransferPendingCode:
			// Set backoff without changing status
			bm.backoffFactor = bm.backoffFactor * 2
			entry, err = Drive(ctx, bm)
			if err != nil {
				return entry, fmt.Errorf(
					"failed to drive transaction from pending to paid: %w",
					err,
				)
			}
		default:
			// Status unknown. includes TransferFailedCode
			entry, err = bm.writeNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from zebpay for transaction: %v",
				entry,
			)
		}
	} else {
		// submit the transaction
		btr, err := bm.client.BulkTransfer(ctx, &zebpay.ClientOpts{
			APIKey: bm.apiKey, SigningKey: bm.signingKey,
		}, zebpay.NewBulkTransferRequest(
			zebpay.NewTransfer(bm.transaction.GenerateIdempotencyKey(), bm.transaction.From, bm.transaction.Amount, bm.transaction.To)))
		if err != nil {
			return nil, fmt.Errorf("failed to check transaction status: %w", err)
		}

		if strings.ToUpper(btr.Data) != "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED" {
			// Status unknown. Fail
			entry, err = bm.writeNextState(ctx, Failed)
			if err != nil {
				return nil, fmt.Errorf("failed to write next state: %w", err)
			}
			return nil, fmt.Errorf(
				"received unknown status from zebpay for transaction: %v",
				entry,
			)
		}

		entry, err = bm.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}

		// Set initial backoff
		bm.backoffFactor = 2
		entry, err = Drive(ctx, bm)
		if err != nil {
			return entry, fmt.Errorf(
				"failed to drive transaction from pending to paid: %w",
				err,
			)
		}
	}
	return bm.transaction, nil
}

// Fail implements TxStateMachine for the Zebpay machine.
func (bm *ZebpayMachine) Fail(ctx context.Context) (*AuthenticatedPaymentState, error) {
	return bm.writeNextState(ctx, Failed)
}
