package main

import (
	"github.com/google/uuid"
)

/*
	Supported resource types:
		DEPOSIT_CRYPTO
		SUPPORT_TICKET
		KYC_LEVEL
		DEPOSIT_FIAT
		ACCOUNT_FROZEN
*/

type emailPayload struct {
	Email        string                 `json:"email"`
	ClientUserID string                 `json:"clientUserId"`
	AccountID    int64                  `json:"accountId"`
	Timestamp    int64                  `json:"timestamp"`
	UUID         uuid.UUID              `json:"uuid"`
	ResourceType string                 `json:"resourceType"`
	Data         map[string]interface{} `json:"data"`
}
