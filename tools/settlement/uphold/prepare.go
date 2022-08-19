package uphold

import (
	"github.com/brave-intl/bat-go/libs/custodian"
	"github.com/shopspring/decimal"
)

// GroupSettlements groups settlements under a single wallet provider id so that we can impose limits based on price
// no signing here, just grouping settlements under a single deposit id
func GroupSettlements(
	settlements *[]custodian.Transaction,
) map[string][]custodian.Transaction {
	groupedByWalletProviderID := make(map[string][]custodian.Transaction)

	for _, payout := range *settlements {
		if payout.WalletProvider == "uphold" {
			walletProviderID := payout.WalletProviderID
			if groupedByWalletProviderID[walletProviderID] == nil {
				groupedByWalletProviderID[walletProviderID] = []custodian.Transaction{}
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
func FlattenPaymentsByWalletProviderID(settlements *[]custodian.Transaction) []custodian.Transaction {
	groupedSettlements := GroupSettlements(settlements)
	flattenedSettlements := []custodian.Transaction{}
	for _, v := range groupedSettlements {
		var (
			flattenedSettlement      custodian.Transaction
			flattenedSettlementProbi decimal.Decimal = decimal.NewFromFloat(0.0)
		)
		for _, record := range v {
			if (flattenedSettlement == custodian.Transaction{}) {
				flattenedSettlement = record
			}
			flattenedSettlementProbi = flattenedSettlementProbi.Add(record.Probi)
		}
		flattenedSettlement.Probi = flattenedSettlementProbi
		flattenedSettlements = append(flattenedSettlements, flattenedSettlement)
	}
	return flattenedSettlements
}
