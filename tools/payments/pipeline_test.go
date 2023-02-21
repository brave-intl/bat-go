package payments

import (
	"context"
	"sync"
	"testing"
)

// TestPipeline validates that the pipeline works appropriately
func TestPipeline(t *testing.T) {
	// make a big list of transactions
	txs := []*Tx{}
	for i := 0; i < 500000; i++ {
		txs = append(txs, &Tx{})
	}

	count, mu := 0, new(sync.Mutex)
	if err := pipeline(context.Background(), 100, len(txs), func(*Tx) error {
		mu.Lock()
		defer mu.Unlock()
		count++
		return nil
	}, txs...); err != nil {
		t.Errorf("failed to pipeline txs: %s", err.Error())
	}

	if count != len(txs) {
		t.Errorf("pipeline txs do not match result: %d", count)
	}
}
