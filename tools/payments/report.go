package payments

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/shopspring/decimal"
)

// A Report is a list of transactions to be loaded into the payments service.
type Report []Tx

// populateStats implements report interface for Report
func (r *Report) populateStats() {}

// An AttestedReport is a list of transactions or report that have been attested by an enclave.
type AttestedReport struct {
	report []AttestedTx
	stats  map[Custodian]custodianStats
	m      sync.RWMutex
}

// UnmarshalJSON implements custom json unmarshaling for attested report
func (ar *AttestedReport) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &ar.report); err != nil {
		return err
	}
	return nil
}

// populateStats implements the report interface, updating the stats of the report
func (ar *AttestedReport) populateStats() {
	ar.m.Lock()
	defer ar.m.Unlock()

	if ar.stats == nil {
		ar.stats = make(map[Custodian]custodianStats)
	}

	for _, v := range ar.report {
		if _, ok := ar.stats[v.Custodian]; !ok {
			// add this custodian to the stats
			ar.stats[v.Custodian] = custodianStats{}
		}

		if stats, ok := ar.stats[v.Custodian]; ok {
			stats.AmountBAT.Add(v.Amount)
			stats.Transactions += 1
			ar.stats[v.Custodian] = stats
		}
	}
}

// SumBAT returns a decimal value for the amount of total bat for the provided custodian(s) in the report
func (ar AttestedReport) SumBAT(custodians ...Custodian) decimal.Decimal {
	ar.m.RLock()
	defer ar.m.RUnlock()
	if len(custodians) < 1 {
		custodians = append(custodians, uphold, gemini, bitflyer)
	}

	total := decimal.Zero
	for _, c := range custodians {
		for k, v := range ar.stats {
			if strings.EqualFold(k.String(), c.String()) {
				total = total.Add(v.AmountBAT)
			}
		}
	}
	return total
}

// Length returns the number of transactions in the report by custodian
func (ar AttestedReport) Length(custodians ...Custodian) uint64 {
	ar.m.RLock()
	defer ar.m.RUnlock()
	if len(custodians) < 1 {
		custodians = append(custodians, uphold, gemini, bitflyer)
	}

	total := uint64(0)
	for _, c := range custodians {
		for k, v := range ar.stats {
			if strings.EqualFold(k.String(), c.String()) {
				total += v.Transactions
			}
		}
	}
	return total
}

// report is an interface describing common report methods
type report interface {
	populateStats()
}

// ReadReport reads a report from the reader
func ReadReport(report report, reader io.Reader) error {
	if err := json.NewDecoder(reader).Decode(report); err != nil {
		return fmt.Errorf("failed to parse report: %w", err)
	}
	report.populateStats()
	return nil
}

const submitWorkerCount = 1000

// Submit performs a submission of approval from an operator to the settlement client
func (ar *AttestedReport) Submit(key, client SettlementClient) error {
	signer := nil // TODO: parameterized signer here
	if err := pipeline(
		func(item interface{}) error {
			if tx, ok := item.(*AttestedTx); ok {
				return client.SubmitTransaction(nil, tx)
			}
		}); err != nil {
		return err
	}
}

// Prepare performs a preparation of transactions for a payout to the settlement client
func (r *Report) Prepare(client SettlementClient) error {
	if err := pipeline(
		func(item interface{}) error {
			if tx, ok := item.(*Tx); ok {
				return client.PrepareTransaction(nil, tx)
			}
		}); err != nil {
		return err
	}
	return nil
}
