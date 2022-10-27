package skus

import (
	"encoding/json"
	"fmt"

	"github.com/brave-intl/bat-go/libs/ptr"
	uuid "github.com/satori/go.uuid"
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

// SigningOrderRequest - the structure of a signing order request
type SigningOrderRequest struct {
	RequestID string         `json:"request_id"`
	Data      []SigningOrder `json:"data"`
}

// SigningOrder - signing order structure
type SigningOrder struct {
	AssociatedData []byte   `json:"associated_data"`
	BlindedTokens  []string `json:"blinded_tokens"`
	IssuerType     string   `json:"issuer_type"`
	IssuerCohort   int16    `json:"issuer_cohort"`
}

// Metadata - skus metadata structure
type Metadata struct {
	ItemID         uuid.UUID `json:"itemId"`
	OrderID        uuid.UUID `json:"orderId"`
	IssuerID       uuid.UUID `json:"issuerId"`
	CredentialType string    `json:"credential_type"`
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
                        {"name": "associated_data", "type": "bytes", "doc": "contains METADATA"},
                        {"name": "valid_to", "type": ["null", "string"], "default": null},
                        {"name": "valid_from", "type": ["null", "string"], "default": null},
                        {
                            "name": "blinded_tokens",
                            "type": {"type" : "array", "items": {"type": "string"}},
                            "default": []
                        }
                    ]
                }
            }
        }
    ]
}`

// SigningOrderResult - structure of a signing result
type SigningOrderResult struct {
	RequestID string        `json:"request_id"`
	Data      []SignedOrder `json:"data"`
}

// SignedOrder - structure for a signed order
type SignedOrder struct {
	PublicKey      string            `json:"public_key"`
	Proof          string            `json:"proof"`
	Status         SignedOrderStatus `json:"status"`
	SignedTokens   []string          `json:"signed_tokens"`
	BlindedTokens  []string          `json:"blinded_tokens"`
	ValidTo        *UnionNullString  `json:"valid_to"`
	ValidFrom      *UnionNullString  `json:"valid_from"`
	AssociatedData []byte            `json:"associated_data"`
}

// SignedOrderStatus - signed order status structure
type SignedOrderStatus int

const (
	// SignedOrderStatusOk - Okay status from signed order status
	SignedOrderStatusOk SignedOrderStatus = iota
	// SignedOrderStatusInvalidIssuer - invalid issuer
	SignedOrderStatusInvalidIssuer
	// SignedOrderStatusError - error status for signed order status
	SignedOrderStatusError
)

// MarshalJSON - marshaller for signed order status
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

// UnmarshalJSON - unmarshaller for signed order status
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

// String - stringer for signed order status
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

// UnionNullString - type describing
type UnionNullString map[string]interface{}

// UnmarshalJSON - implement unmarshaling for union null string
func (u *UnionNullString) UnmarshalJSON(data []byte) error {
	var temp map[string]interface{}
	err := json.Unmarshal(data, &temp)
	if err != nil {
		return fmt.Errorf("error deserializing union: %w", err)
	}
	*u = temp
	return nil
}

// Value - perform a valuer on unionnullstring
func (u UnionNullString) Value() *string {
	s, ok := u["string"]
	if ok {
		return ptr.FromString(s.(string))
	}
	_, ok = u["null"]
	if ok {
		return nil
	}
	panic("unknown value for union")
}
