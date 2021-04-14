package referral

import (
	"errors"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
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
		"referral",
		avro.ParseCodecs(schemas),
		attemptDecodeList,
	)
}

// DecodeBatch decodes a batch of messages
func DecodeBatch(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
	modifiers ...map[string]string,
) (*[]models.ConvertableTransaction, error) {
	txs := []models.ConvertableTransaction{}
	for _, msg := range msgs {
		var referral models.Referral
		if err := avro.TryDecode(codecs, attemptDecodeList, msg, &referral); err != nil {
			return nil, err
		}
		for _, modifier := range modifiers {
			value := modifier[referral.CountryGroupID]
			if value == "" {
				return nil, errors.New("the country code was not found in the modifiers")
			}
			probi, err := decimal.NewFromString(value)
			if err != nil {
				return nil, err
			}
			referral.Probi = probi
		}
		txs = append(txs, models.ConvertableTransaction(&referral))
	}
	return &txs, nil
}
