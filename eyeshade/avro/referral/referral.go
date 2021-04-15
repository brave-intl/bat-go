package referral

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	latestSchemaKey   = "v1"
	attemptDecodeList = []string{latestSchemaKey}
	schemas           = map[string]string{
		latestSchemaKey: `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "referral",
			"doc": "This message is sent when a referral is finalized by a service",
			"fields": [
				{ "name": "transactionId", "type": "string" },
				{ "name": "channelId", "type": "string" },
				{ "name": "ownerId", "type": "string" },
				{ "name": "finalizedTimestamp", "type": "string" },
				{ "name": "referralCode", "type": "string", "default": "" },
				{ "name": "downloadId", "type": "string" },
				{ "name": "downloadTimestamp", "type": "string" },
				{ "name": "countryGroupId", "type": "string", "default": "" },
				{ "name": "platform", "type": "string" }
			]
		}`,
	}
)

// New holds all info needed to create a referral parser
func New() *avro.Handler {
	return avro.NewHandler(
		avro.TopicKeys.Referral,
		avro.ParseCodecs(schemas),
		attemptDecodeList,
	)
}

// DecodeBatch decodes a batch of messages
func DecodeBatch(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
) (*[]models.Referral, error) {
	referrals := []models.Referral{}
	for _, msg := range msgs {
		var referral models.Referral
		if err := avro.TryDecode(codecs, attemptDecodeList, msg, &referral); err != nil {
			return nil, err
		}
		referrals = append(referrals, referral)
	}
	return &referrals, nil
}
