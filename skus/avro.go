package skus

import (
	"encoding/json"
	"fmt"
)

const voteSchema = `{
  "namespace": "brave.payments",
  "type": "record",
  "name": "vote",
  "doc": "This message is sent when a user funded wallet has successfully auto-contributed to a channel",
  "fields": [
    { "name": "id", "type": "string" },
    { "name": "type", "type": "string" },
    { "name": "channel", "type": "string" },
    { "name": "createdAt", "type": "string" },
    { "name": "baseVoteValue", "type": "string", "default":"0.25" },
    { "name": "voteTally", "type": "long", "default":1 },
    { "name": "fundingSource", "type": "string", "default": "uphold" }
  ]
}`

const signingOrderRequestSchema = `{
    "namespace": "brave.payments",
    "type": "record",
    "doc": "Top level request containing the data to be processed, as well as any top level metadata for this message.",
    "name": "signingOrderRequestSchema",
    "fields" : [
        {"name": "request_id", "type": "string"},
        {
            "name": "data",
            "type": {
                "type": "array",
                "items": {
                    "namespace": "brave.payments",
                    "type": "record",
                    "name": "SigningOrder",
                    "fields": [
                        {"name": "associated_data", "type": "bytes", "doc": "contains METADATA"},
                        {
                            "name": "blinded_tokens",
                            "type": {
                                "type": "array",
                                "items": {
                                    "name": "blinded_token",
                                    "type": "string",
                                    "namespace": "brave.payments"
                                }
                            }
                        },
                        {"name": "issuer_type", "type": "string"},
                        {"name": "issuer_cohort", "type": "int"}
                    ]
                }
            }
        }
    ]
}`

type SigningOrderRequest struct {
	RequestID string         `json:"request_id"`
	Data      []SigningOrder `json:"data"`
}

type SigningOrder struct {
	AssociatedData []byte   `json:"associated_data"`
	BlindedTokens  []string `json:"blinded_tokens"`
	IssuerType     string   `json:"issuer_type"`
	IssuerCohort   int      `json:"issuer_cohort"`
}

const signingOrderResultSchema = `{
    "namespace": "brave.payments",
    "type": "record",
    "doc": "Top level request containing the data to be processed, as well as any top level metadata for this message.",
    "name": "signingOrderResultSchema",
    "fields" : [
        {"name": "request_id", "type": "string"},
        {
            "name": "data",
            "type": {
                "type": "array",
                "items":{
                    "namespace": "brave.payments",
                    "type": "record",
                    "name": "SignedOrder",
                    "fields": [
                        {
                            "name": "signed_tokens",
                            "type": {
                                "type": "array",
                                "items": {
                                    "name": "signed_token",
                                    "type": "string"
                                }
                            }
                        },
                        {"name": "public_key", "type": "string"},
                        {"name": "proof", "type": "string"},
                        {"name": "status", "type": {
                            "name": "SigningResultStatus",
                            "type": "enum",
                            "symbols": ["ok", "invalid_issuer", "error"]
                        }},
                        {"name": "associated_data", "type": "bytes", "doc": "contains METADATA"}
                    ]
                }
            }
        }
    ]
}`

type SigningOrderResult struct {
	RequestID string        `json:"request_id"`
	Data      []SignedOrder `json:"data"`
}

type SignedOrder struct {
	PublicKey      string            `json:"public_key"`
	Proof          string            `json:"proof"`
	Status         SignedOrderStatus `json:"status"`
	SignedTokens   []string          `json:"signed_tokens"`
	AssociatedData []byte            `json:"associated_data"`
}

type SignedOrderStatus int

const (
	SignedOrderStatusOk SignedOrderStatus = iota
	SignedOrderStatusInvalidIssuer
	SignedOrderStatusError
)

func (s SignedOrderStatus) MarshalJSON() ([]byte, error) {
	var status string
	switch s {
	case SignedOrderStatusOk:
		status = "ok"
	case SignedOrderStatusInvalidIssuer:
		status = "invalid_issuer"
	case SignedOrderStatusError:
		status = "error"
	default:
		return nil, fmt.Errorf("signed order creds marshal error: invalid type %s", status)
	}

	return json.Marshal(status)
}

func (s *SignedOrderStatus) UnmarshalJSON(data []byte) error {
	var str string
	err := json.Unmarshal(data, &str)
	if err != nil {
		return fmt.Errorf("signed order creds unmarshal error: %w", err)
	}

	switch str {
	case "ok":
		*s = SignedOrderStatusOk
	case "invalid_issuer":
		*s = SignedOrderStatusInvalidIssuer
	case "error":
		*s = SignedOrderStatusError
	default:
		return fmt.Errorf("signed order creds unmarshal error: invalid type %s", str)
	}

	return nil
}

func (s SignedOrderStatus) String() string {
	switch s {
	case SignedOrderStatusOk:
		return "ok"
	case SignedOrderStatusInvalidIssuer:
		return "invalid_issuer"
	case SignedOrderStatusError:
		return "error"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}
