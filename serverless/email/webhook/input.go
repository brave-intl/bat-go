package main

/*
	Supported resource types:
		DEPOSIT_CRYPTO
		SUPPORT_TICKET
		KYC_LEVEL
		DEPOSIT_FIAT
		ACCOUNT_FROZEN
*/

type emailPayload struct {
	Email        string                 `json:"email" valid:"email"`
	ClientUserID string                 `json:"clientUserId" valid:"-"`
	AccountID    int64                  `json:"accountId" valid:"-"`
	Timestamp    int64                  `json:"timestamp" valid:"-"`
	UUID         string                 `json:"uuid" valid:"uuid"`
	ResourceType string                 `json:"resourceType" valid:"in(DEPOSIT_CRYPTO|SUPPORT_TICKET|KYC_LEVEL|DEPOSIT_FIAT|ACCOUNT_FROZEN)"`
	Data         map[string]interface{} `json:"data" valid:"-"`
}
