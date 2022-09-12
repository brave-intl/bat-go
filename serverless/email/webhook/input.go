package main

import (
	"github.com/google/uuid"
)

type resourceType string

const (
	depositCryptoResourceType resourceType = "DEPOSIT_CRYPTO"
	supportTicketResourceType resourceType = "SUPPORT_TICKET"
	kycLevelResourceType      resourceType = "KYC_LEVEL"
	depositFiatResourceType   resourceType = "DEPOSIT_FIAT"
	accountFrozenResourceType resourceType = "ACCOUNT_FROZEN"
)

type emailPayload struct {
	Email        string                 `json:"email"`
	ClientUserID string                 `json:"clientUserId"`
	AccountID    int64                  `json:"accountId"`
	Timestamp    int64                  `json:"timestamp"`
	UUID         uuid.UUID              `json:"uuid"`
	ResourceType resourceType           `json:"resourceType"`
	Data         map[string]interface{} `json:"data"`
}
