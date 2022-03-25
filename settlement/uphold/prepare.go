package uphold

import (
	"github.com/brave-intl/bat-go/settlement"
	"github.com/shopspring/decimal"
)

// GroupSettlements groups settlements under a single wallet provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]settlement.Transaction,
) map[string][]settlement.Transaction {
	groupedByWalletProviderID := make(map[string][]settlement.Transaction)

	for _, payout := range *settlements {
		if payout.WalletProvider == "uphold" {
			walletProviderID := payout.WalletProviderID
			if groupedByWalletProviderID[walletProviderID] == nil {
				groupedByWalletProviderID[walletProviderID] = []settlement.Transaction{}
			}
			groupedByWalletProviderID[walletProviderID] = append(groupedByWalletProviderID[walletProviderID], payout)
		}
	}
	return groupedByWalletProviderID
}

// FlattenPaymentsByWalletProviderID returns settlements with any instances where a single
// Uphold wallet is getting multiple payments as a single instance with the amounts
// summed. This decreases the total number of distinct transactions that have to be sent
// without impacting the payout amount for any given user.
func FlattenPaymentsByWalletProviderID(settlements *[]settlement.Transaction) []settlement.Transaction {
	groupedSettlements := GroupSettlements(settlements)
	flattenedSettlements := []settlement.Transaction{}
	for _, v := range groupedSettlements {
		var (
			flattenedSettlement       settlement.Transaction
			flattenedSettlementAmount decimal.Decimal = decimal.NewFromFloat(0.0)
		)
		for _, record := range v {
			if (flattenedSettlement == settlement.Transaction{}) {
				flattenedSettlement = record
			}
			flattenedSettlementAmount = flattenedSettlementAmount.Add(record.Amount)
		}
		flattenedSettlement.Amount = flattenedSettlementAmount
		flattenedSettlements = append(flattenedSettlements, flattenedSettlement)
	}
	return flattenedSettlements
}
