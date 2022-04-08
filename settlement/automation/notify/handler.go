package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/brave-intl/bat-go/utils/logging"
	"sync"

	"github.com/brave-intl/bat-go/settlement/automation/event"
	"github.com/brave-intl/bat-go/utils/backoff"
	"github.com/brave-intl/bat-go/utils/clients/payment"
)

type notify struct {
	redis   *event.Client
	payment payment.Client
	retry   backoff.RetryFunc
	rwMutex *sync.RWMutex
}

func newHandler(redis *event.Client, payment payment.Client, retry backoff.RetryFunc) *notify {
	return &notify{
		redis:   redis,
		payment: payment,
		retry:   retry,
	}
}

// concurrent access required
var transactions []payment.Transaction

func (n *notify) Handle(ctx context.Context, messages []event.Message) error {
	if len(transactions) > 100 {
		err := n.sendTransactionsReadyNotification(ctx)
		if err != nil {
			return fmt.Errorf("error sending transaction ready notification: %w", err)
		}
	}
	err := n.saveTransactions(messages)
	if err != nil {
		return fmt.Errorf("error sending transaction ready notification: %w", err)
	}
	return nil
}

func (n *notify) saveTransactions(messages []event.Message) error {
	n.rwMutex.Lock()
	defer n.rwMutex.Unlock()

	var transaction payment.Transaction
	for _, message := range messages {
		err := json.Unmarshal([]byte(message.Body), &transaction)
		if err != nil {
			// TODO dlq
			fmt.Println("ERROR UNMARSHAL")
			continue
		}
		transactions = append(transactions, transaction)
	}

	return nil
}

func (n *notify) sendTransactionsReadyNotification(ctx context.Context) error {
	n.rwMutex.RLock()
	defer n.rwMutex.RUnlock()

	// For testing purposes
	logging.FromContext(ctx).Info().
		Interface("transactions", transactions).
		Msg("ready settlement transactions")

	transactions = make([]payment.Transaction, 0)

	return nil
}
