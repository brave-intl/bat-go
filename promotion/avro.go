package promotion

const suggestionEventSchema = `{
	"namespace": "brave.grants",
  	"type": "record",
  	"name": "suggestion",
  	"doc": "This message is sent when a client suggests to 'spend' a grant",
  	"fields": [
		{ "name": "id", "type": "string" },
		{ "name": "type", "type": "string" },
		{ "name": "channel", "type": "string" },
		{ "name": "createdAt", "type": "string" },
		{ "name": "totalAmount", "type": "string" },
		{ "name": "orderId", "type": "string", "default": "" },
		{ "name": "funding",
		  "type": {
			"type": "array",
			"items": {
			  "type": "record",
			  "name": "funding",
			  "doc": "This record represents a funding source, currently a promotion.",
			  "fields": [
				{ "name": "type", "type": "string" },
				{ "name": "amount", "type": "string" },
				{ "name": "cohort", "type": "string" },
				{ "name": "promotion", "type": "string" }
			  ]
			}
		  }
		}
  	]}`

const adminAttestationEventSchema = `{
	"type": "record", 
	"name": "DefaultMessage", 
	"fields": [
		{ "name": "wallet_id", "type": "string" },
		{ "name": "service", "type": "string" },
		{ "name": "signal", "type": "string" },
		{ "name": "score", "type": "int" },
		{ "name": "justification", "type": "string" },
		{ "name": "created_at", "type": "string" }
	]}`

// AdminAttestationEvent - kafka admin attestation event
type AdminAttestationEvent struct {
	WalletID      string `json:"wallet_id"`
	Service       string `json:"service"`
	Signal        string `json:"signal"`
	Score         int32  `json:"score"`
	Justification string `json:"justification"`
	CreatedAt     string `json:"created_at"`
}
