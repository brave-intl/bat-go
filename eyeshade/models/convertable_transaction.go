package models

import "sort"

// ConvertableTransaction allows a struct to be converted into a transaction
type ConvertableTransaction interface {
	ToTxs() []Transaction
	Valid() error
	Ignore() bool
	ToTxIDs() []string
}

// ReferralsToConvertableTransactions turns contributions into votes
func ReferralsToConvertableTransactions(referrals ...Referral) []ConvertableTransaction {
	convertableTxs := []ConvertableTransaction{}
	for i := range referrals {
		convertableTxs = append(convertableTxs, &referrals[i])
	}
	return convertableTxs
}

// SettlementsToConvertableTransactions turns contributions into votes
func SettlementsToConvertableTransactions(settlements ...Settlement) []ConvertableTransaction {
	convertableTxs := []ConvertableTransaction{}
	for i := range settlements {
		convertableTxs = append(convertableTxs, &settlements[i])
	}
	return convertableTxs
}

// CollectTransactions converts and orders transactions by their id
func CollectTransactions(convertableTxs ...ConvertableTransaction) []Transaction {
	ids := []string{}
	hash := map[string]Transaction{}
	for _, convertableTx := range convertableTxs {
		ids = append(ids, convertableTx.ToTxIDs()...)
		converted := convertableTx.ToTxs()
		// txs = append(txs, converted...)
		for _, convert := range converted {
			hash[convert.ID] = convert
		}
	}

	txs := []Transaction{}
	sort.Strings(ids)
	for _, id := range ids {
		txs = append(txs, hash[id])
	}
	return txs
}
