package payments

import "encoding/json"

var (
	status0, _ = json.Marshal(QLDBPaymentTransitionData{Status: 0})
	status1, _ = json.Marshal(QLDBPaymentTransitionData{Status: 1})
	status2, _ = json.Marshal(QLDBPaymentTransitionData{Status: 2})
	status3, _ = json.Marshal(QLDBPaymentTransitionData{Status: 3})
	status4, _ = json.Marshal(QLDBPaymentTransitionData{Status: 4})
	status5, _ = json.Marshal(QLDBPaymentTransitionData{Status: 5})
)

var transactionHistorySetTrue = [][]QLDBPaymentTransitionHistoryEntry{
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status1}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status2}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status3}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status4}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status1}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status2}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status3}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status1}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status2}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status1}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
	},
}

var transactionHistorySetFalse = [][]QLDBPaymentTransitionHistoryEntry{
	// Transitions must always start at 0
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status1}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status2}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status3}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status4}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status4}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status5}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status4}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status3}},
	},
	{
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status0}},
		{Data: QLDBPaymentTransitionHistoryEntryData{Data: status2}},
	},
}

var upholdCreateTransactionSuccessResponse = map[string]interface{}{
	"application": nil,
	"createdAt":   "2018-08-01T09:53:47.020Z",
	"denomination": map[string]string{
		"amount":   "5.00",
		"currency": "GBP",
		"pair":     "GBPUSD",
		"rate":     "1.31",
	},
	"destination": map[string]interface{}{
		"CardId":      "bc9b3911-4bc1-4c6d-ac05-0ae87dcfc9b3",
		"amount":      "5.57",
		"base":        "5.61",
		"commission":  "0.04",
		"currency":    "EUR",
		"description": "Angel Rath",
		"fee":         "0.00",
		"isMember":    true,
		"node": map[string]interface{}{
			"id":   "bc9b3911-4bc1-4c6d-ac05-0ae87dcfc9b3",
			"type": "card",
			"user": map[string]string{
				"id": "21e65c4d-55e4-41be-97a1-ff38d8f3d945",
			},
		},
		"rate": "0.85620",
		"type": "card",
	},
	"fees": []map[string]string{
		{
			"amount":     "0.04",
			"currency":   "EUR",
			"percentage": "0.65",
			"target":     "destination",
			"type":       "exchange",
		},
	},
	"id":      "2c326b15-7106-48be-a326-06f19e69746b",
	"message": "null",
	"network": "uphold",
	"normalized": []map[string]string{
		{
			"amount":     "6.56",
			"commission": "0.05",
			"currency":   "USD",
			"fee":        "0.00",
			"rate":       "1.00000",
			"target":     "destination",
		},
	},
	"origin": map[string]interface{}{
		"CardId":      "48ce2ac5-c038-4426-b2f8-a2bdbcc93053",
		"amount":      "6.56",
		"base":        "6.56",
		"commission":  "0.00",
		"currency":    "USD",
		"description": "Angel Rath",
		"fee":         "0.00",
		"isMember":    true,
		"node": map[string]interface{}{
			"id":   "48ce2ac5-c038-4426-b2f8-a2bdbcc93053",
			"type": "card",
			"user": map[string]string{
				"id": "21e65c4d-55e4-41be-97a1-ff38d8f3d945",
			},
		},
		"rate": "1.16795",
		"sources": []map[string]string{
			{
				"amount": "6.56",
				"id":     "3db4ef24-c529-421f-8e8f-eb9da1b9a582",
			},
		},
		"type": "card",
	},
	"params": map[string]interface{}{
		"currency": "USD",
		"margin":   "0.65",
		"pair":     "EURUSD",
		"progress": "1",
		"rate":     "1.16795",
		"ttl":      18000,
		"type":     "transfer",
	},
	"priority":  "normal",
	"reference": nil,
	"Status":    "completed",
	"type":      "transfer",
}

var upholdCommitTransactionSuccessResponse = upholdCreateTransactionSuccessResponse
var upholdCommitTransactionFailureResponse = upholdCreateTransactionSuccessResponse
var upholdCreateTransactionFailureResponse = upholdCreateTransactionSuccessResponse

var geminiBulkPaySuccessResponse = []map[string]string{
	{
		"result":      "ok",
		"tx_ref":      "",
		"amount":      "",
		"currency":    "",
		"destination": "",
		"Status":      "",
		"reason":      "",
	},
}

var geminiBulkPayFailureResponse = []map[string]string{
	{
		"result":      "error",
		"tx_ref":      "",
		"amount":      "",
		"currency":    "",
		"destination": "",
		"Status":      "",
		"reason":      "",
	},
}

var geminiTransactionCheckSuccessResponse = map[string]string{
	"result":      "ok",
	"tx_ref":      "",
	"amount":      "",
	"currency":    "",
	"destination": "",
	"Status":      "",
	"reason":      "",
}

var geminiTransactionCheckFailureResponse = map[string]string{
	"result":      "error",
	"tx_ref":      "",
	"amount":      "",
	"currency":    "",
	"destination": "",
	"Status":      "",
	"reason":      "",
}

var bitflyerTransactionSubmitSuccessResponse = map[string]interface{}{
	"dry_run": "false",
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	},
	},
}

var bitflyerTransactionSubmitFailureResponse = map[string]interface{}{
	"dry_run": "false",
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	},
	},
}

var bitflyerTransactionCheckStatusSuccessResponse = map[string]interface{}{
	"dry_run": "false",
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	},
	},
}

var bitflyerTransactionCheckStatusFailureResponse = map[string]interface{}{
	"dry_run": "false",
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	},
	},
}
