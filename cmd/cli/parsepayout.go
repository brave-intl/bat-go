package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/payments/pb"
)

// parsePayoutFile - parse the antifraud payout report
func parsePayoutFile(f string) ([]*pb.Transaction, error) {
	fd, err := os.Open(f)
	if err != nil {
		return nil, fmt.Errorf("failed to open payout file: %w", err)
	}
	defer func() {
		if err := fd.Close(); err != nil {
			panic(fmt.Sprintf("failed to close file: %s", err.Error()))
		}
	}()

	var txs = []*pb.Transaction{}
	err = json.NewDecoder(fd).Decode(&txs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payout file: %w", err)
	}

	return txs, nil
}
