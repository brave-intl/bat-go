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
	ResourceType string                 `json:"resourceType" valid:"in(DEPOSIT_CRYPTO|SUPPORT_TICKET|KYC_LEVEL|DEPOSIT_FIAT|ACCOUNT_FROZEN|PASSWORD_RESET|WITHDRAWAL_CRYPTO)"`
	Data         map[string]interface{} `json:"data" valid:"-"`
}

func (ep *emailPayload) SesTemplateFromResourceType() string {
	switch ep.ResourceType {
	case "DEPOSIT_CRYPTO":
		return "Deposit_Crypto"
	case "SUPPORT_TICKET":
		return "Support_Ticket"
	case "KYC_LEVEL":
		return "KYC_Level"
	case "DEPOSIT_FIAT":
		return "Deposit_Fiat"
	case "ACCOUNT_FROZEN":
		return "Account_Frozen"
	case "PASSWORD_RESET":
		return "Password_Reset"
	case "WITHDRAWAL_CRYPTO":
		return "Withdrawal_Crypto"
	default:
		return ""
	}

}
