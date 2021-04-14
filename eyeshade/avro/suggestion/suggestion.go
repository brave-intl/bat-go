package settlement

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	latest            = "v2"
	attemptDecodeList = []string{latest, "v1"}
	schemas           = map[string]string{
		"v1": `{
			"namespace": "brave.grants",
			"type": "record",
			"name": "suggestion",
			"doc": "This message is sent when a client suggests to \"spend\" a grant",
			"fields": [
				{ "name": "id", "type": "string" },
				{ "name": "type", "type": "string" },
				{ "name": "channel", "type": "string" },
				{ "name": "createdAt", "type": "string" },
				{ "name": "totalAmount", "type": "string" },
				{
					"name": "funding",
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
			]
		}`,
		latest: `{
			"namespace": "brave.grants",
			"type": "record",
			"name": "suggestion",
			"doc": "This message is sent when a client suggests to \"spend\" a grant",
			"fields": [
				{ "name": "id", "type": "string" },
				{ "name": "type", "type": "string" },
				{ "name": "channel", "type": "string" },
				{ "name": "createdAt", "type": "string" },
				{ "name": "totalAmount", "type": "string" },
				{ "name": "orderId", "type": "string", "default": "" },
				{
					"name": "funding",
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
			]
		}`,
	}
)

// New holds all info needed to create a suggestion parser
func New() *avro.Handler {
	return avro.NewHandler(
		"suggestion",
		avro.ParseCodecs(schemas),
		attemptDecodeList,
	)
}

// DecodeBatch decodes a batch of messages
func DecodeBatch(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
) (*[]models.Vote, error) {
	votes := []models.Vote{}
	for _, msg := range msgs {
		var suggestion models.Suggestion
		if err := avro.TryDecode(codecs, attemptDecodeList, msg, &suggestion); err != nil {
			return nil, err
		}
		votes = append(votes, models.Vote(&suggestion))
	}
	return &votes, nil
}
