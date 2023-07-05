package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/libs/altcurrency"
	"github.com/brave-intl/bat-go/libs/clients/bitflyer"
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/brave-intl/bat-go/tools/settlement"
	bitflyersettlement "github.com/brave-intl/bat-go/tools/settlement/bitflyer"
	bitflyercmd "github.com/brave-intl/bat-go/tools/settlement/cmd"
	"github.com/shopspring/decimal"
)

// BitflyerMachine is an implementation of TxStateMachine for Bitflyer's use-case.
// Including the baseStateMachine provides a default implementation of TxStateMachine,
type BitflyerMachine struct {
	baseStateMachine
}

// Pay implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Pay(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	var (
		entry                 *Transaction
		submittedTransactions map[string][]custodian.Transaction
	)
	if bm.transaction.State == Pending {
		// Get status of transaction and update the state accordingly
		return bm.writeNextState(ctx, Paid)
	} else {
		// Submit formatted transaction
		bitflyerClient, err := bitflyercmd.GetBitflyerAuthorizedClient(ctx, "")
		aggregateTransaction := transactionToSettlementAggregateTransaction(bm.transaction)
		aggregateTransactionSet := []settlement.AggregateTransaction{aggregateTransaction}
		request, err := getBitflyerRequest(ctx, aggregateTransactionSet)
		if err != nil {
			return nil, fmt.Errorf("failed to get bitflyer request: %w", err)
		}
		submittedTransactions, err = bitflyersettlement.SubmitBulkPayoutTransactions(
			ctx,
			aggregateTransactionSet,
			submittedTransactions,
			*request,
			bitflyerClient,
			1, // Hard code number of transactions, as we will only do one at a time
			1, // Hard code progress, as there is only one
		)
		if err != nil {
			return nil, fmt.Errorf("failed to submit bulk payout transactions: %w", err)
		}
		entry, err = bm.writeNextState(ctx, Pending)
		if err != nil {
			return nil, fmt.Errorf("failed to write next state: %w", err)
		}
		entry, err = Drive(ctx, bm)
		if err != nil {
			return nil, fmt.Errorf("failed to drive transaction from pending to paid: %w", err)
		}
	}
	return entry, nil
}

// Fail implements TxStateMachine for the Bitflyer machine.
func (bm *BitflyerMachine) Fail(ctx context.Context) (*Transaction, error) {
	/*if !bm.transaction.shouldDryRun() {
		// Do bitflyer stuff
	}*/
	return bm.writeNextState(ctx, Failed)
}

func getBitflyerRequest(ctx context.Context, aggregateTransactions []settlement.AggregateTransaction) (*bitflyer.WithdrawToDepositIDBulkPayload, error) {
	bitflyerClient, err := bitflyercmd.GetBitflyerAuthorizedClient(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get bitflyer authorized client: %w", err)
	}
	//  this will only fetch a new quote when needed - but ensures that we don't have problems due to quote expiring midway through
	quote, err := bitflyerClient.FetchQuote(ctx, "BAT_JPY", true)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bitflyer quote: %w", err)
	}

	request, err := bitflyersettlement.CreateBitflyerRequest(
		nil, // Ignore dry run settings for now. We handle it ourselves.
		quote.PriceToken,
		aggregateTransactions,
	)
	if err != nil {
		return nil, err
	}
	return request, nil
}

func transactionToSettlementAggregateTransaction(transaction *Transaction) settlement.AggregateTransaction {
	altCurrencyBAT := altcurrency.BAT
	custodianTransaction := custodian.Transaction{
		AltCurrency:      &altCurrencyBAT,
		Authority:        "",
		Amount:           decimal.New(1, 1),
		ExchangeFee:      decimal.New(1, 1),
		FailureReason:    "",
		Currency:         "",
		Destination:      "",
		Publisher:        "",
		BATPlatformFee:   decimal.New(1, 1),
		Probi:            decimal.New(1, 1),
		ProviderID:       "",
		WalletProvider:   "",
		WalletProviderID: "",
		Channel:          "",
		SignedTx:         "",
		Status:           "",
		SettlementID:     "",
		TransferFee:      decimal.New(1, 1),
		Type:             "",
		ValidUntil:       time.Now(),
		DocumentID:       "",
		Note:             "",
	}
	aggregateTransaction := settlement.AggregateTransaction{
		Transaction: custodianTransaction,
		Inputs:      []custodian.Transaction{custodianTransaction},
		SourceFrom:  "",
	}
	return aggregateTransaction
}
