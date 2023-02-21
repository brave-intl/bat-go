package payments

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/shopspring/decimal"
)

func SumBAT[T isTransaction](txs ...T) decimal.Decimal {
	total := decimal.Zero
	for _, v := range txs {
		total = total.Add(v.GetAmount())
	}
	return total
}

// AttestedReport is the report of payouts after being prepared
type AttestedReport []*AttestedTx

// PreparedReport is the report of payouts prior to being prepared
type PreparedReport []*PrepareTx

// ReadReport reads a report from the reader
func ReadReport(report any, reader io.Reader) error {
	if err := json.NewDecoder(reader).Decode(report); err != nil {
		return fmt.Errorf("failed to parse report: %w", err)
	}
	return nil
}

const submitWorkerCount = 1000

// Submit performs a submission of approval from an operator to the settlement client
func (r AttestedReport) Submit(ctx context.Context, key ed25519.PrivateKey, client SettlementClient) error {
	signer := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(key.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"host",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: key,
		Opts:     crypto.Hash(0),
	}

	if err := pipeline(ctx, submitWorkerCount, len(r),
		func(tx *AttestedTx) error {
			return client.SubmitTransaction(ctx, signer, tx)
		}, r...); err != nil {
		return err
	}
	return nil
}

const prepareWorkerCount = 1000

// Prepare performs a preparation of transactions for a payout to the settlement client
func (r PreparedReport) Prepare(ctx context.Context, client SettlementClient) error {
	if err := pipeline(ctx, prepareWorkerCount, len(r),
		func(tx *PrepareTx) error {
			return client.PrepareTransaction(ctx, tx)
		}, r...); err != nil {
		return err
	}
	return nil
}
