package payments

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

/*var geminiBulkPayFailureResponse = []map[string]string{
	{
		"result":      "error",
		"tx_ref":      "",
		"amount":      "",
		"currency":    "",
		"destination": "",
		"Status":      "",
		"reason":      "",
	},
}*/

var geminiTransactionCheckSuccessResponse = map[string]string{
	"result":      "ok",
	"tx_ref":      "",
	"amount":      "",
	"currency":    "",
	"destination": "",
	"Status":      "",
	"reason":      "",
}

/*var geminiTransactionCheckFailureResponse = map[string]string{
	"result":      "error",
	"tx_ref":      "",
	"amount":      "",
	"currency":    "",
	"destination": "",
	"Status":      "",
	"reason":      "",
}*/

var bitflyerTransactionSubmitSuccessResponse = map[string]interface{}{
	"dry_run": false,
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "pending",
		"transfer_id":     "",
	}},
}

var bitflyerTransactionSubmitFailureResponse = map[string]interface{}{
	"dry_run": false,
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	}},
}

var bitflyerTransactionCheckStatusSuccessResponse = map[string]interface{}{
	"dry_run": false,
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "success",
		"transfer_id":     "",
	}},
}

var bitflyerTransactionCheckStatusSuccessResponsePending = map[string]interface{}{
	"dry_run": false,
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "pending",
		"transfer_id":     "",
	}},
}

var bitflyerTransactionCheckStatusFailureResponse = map[string]interface{}{
	"dry_run": false,
	"withdrawals": []map[string]interface{}{{
		"currency_code":   "",
		"amount":          1.0,
		"message":         "",
		"transfer_Status": "",
		"transfer_id":     "",
	}},
}

var bitflyerTransactionTokenRefreshResponse = map[string]interface{}{
	"dry_run":      false,
	"access_token": "Look at me. I'm a token.",
	"refresh_toke": "another token",
	"expires_in":   4,
	"scope":        "some scope",
	"account_hash": "hashed something",
	"tokey_type":   "token type",
}

var bitflyerFetchPriceResponse = map[string]interface{}{
	"product_code":  "BAT_JPY",
	"main_currency": "BAT",
	"sub_currency":  "",
	"rate":          4,
	"price_token":   "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJPbmxpbmUgSldUIEJ1aWxkZXIiLCJpYXQiOjE2OTM1MTczODksImV4cCI6MTg1MTI4Mzc4OSwiYXVkIjoidGVzdCIsInN1YiI6InRlc3QifQ.6lcVSDtmVJcix01cn2wf3maXUyoGwAWn_hXQTLQtK40",
}

var zebpayTransactionSubmitSuccessResponse = map[string]interface{}{
	"data": "ALL_SENT_TRANSACTIONS_ACKNOWLEDGED",
}

var zebpayTransactionSubmitFailureResponse = map[string]interface{}{
	"data": "oops",
}

var zebpayTransactionCheckStatusSuccessResponse = map[string]interface{}{
	"transaction_id": "725c920b-d158-56fb-b5cf-5910d9ca4a16",
	"code":           2,
	"status":         "Success",
	"details": map[string]interface{}{
		"amount":      13.736461457342187,
		"destination": 512,
	},
}

var zebpayTransactionCheckStatusSuccessResponsePending = map[string]interface{}{
	"transaction_id": "725c920b-d158-56fb-b5cf-5910d9ca4a16",
	"code":           1,
	"status":         "Pending",
	"details": map[string]interface{}{
		"amount":      13.736461457342187,
		"destination": 512,
	},
}

var zebpayTransactionCheckStatusFailureResponse = map[string]interface{}{
	"transaction_id": "725c920b-d158-56fb-b5cf-5910d9ca4a16",
	"code":           3,
	"status":         "Failed",
	"details": map[string]interface{}{
		"amount":      13.736461457342187,
		"destination": 512,
	},
}
