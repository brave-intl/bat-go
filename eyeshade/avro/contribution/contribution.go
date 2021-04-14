package settlement

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	latest            = "v1"
	attemptDecodeList = []string{latest}
	schemas           = map[string]string{
		latest: `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "vote",
			"doc": "This message is sent when a user funded wallet has successfully auto-contributed to a channel",
			"fields": [
				{ "name": "id", "type": "string" },
				{ "name": "type", "type": "string" },
				{ "name": "channel", "type": "string" },
				{ "name": "createdAt", "type": "string" },
				{ "name": "baseVoteValue", "type": "string", "default": "0.25" },
				{ "name": "voteTally", "type": "long", "default": 1 },
				{ "name": "fundingSource", "type": "string", "default": "uphold" }
			]
		}`,
	}
)

// New holds all info needed to create a contribution parser
func New() *avro.Handler {
	return avro.NewHandler(
		avro.TopicKeys.Contribution,
		avro.ParseCodecs(schemas),
		attemptDecodeList,
	)
}

// DecodeBatch decodes a batch of messages
func DecodeBatch(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
) (*[]models.Vote, error) {
	contributions := []models.Vote{}
	for _, msg := range msgs {
		var contribution models.Contribution
		if err := avro.TryDecode(codecs, attemptDecodeList, msg, &contribution); err != nil {
			return nil, err
		}
		contributions = append(contributions, models.Vote(&contribution))
	}
	return &contributions, nil
}
