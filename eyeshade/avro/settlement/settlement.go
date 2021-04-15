package settlement

import (
	"time"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	latest            = "v3"
	attemptDecodeList = []string{latest, "v2", "v1"}
	schemas           = map[string]string{
		"v1": `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "payout",
			"doc": "This message is sent when settlement message is to be sent",
			"fields": [
				{ "name": "address", "type": "string" },
				{ "name": "settlementId", "type": "string" },
				{ "name": "publisher", "type": "string" },
				{ "name": "altcurrency", "type": "string" },
				{ "name": "currency", "type": "string" },
				{ "name": "owner", "type": "string" },
				{ "name": "probi", "type": "string" },
				{ "name": "amount", "type": "string" },
				{ "name": "fee", "type": "string" },
				{ "name": "commission", "type": "string" },
				{ "name": "fees", "type": "string" },
				{ "name": "type", "type": "string" }
			]
		}`,
		"v2": `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "payout",
			"doc": "This message is sent when settlement message is to be sent",
			"fields": [
				{ "name": "address", "type": "string" },
				{ "name": "settlementId", "type": "string" },
				{ "name": "publisher", "type": "string" },
				{ "name": "altcurrency", "type": "string" },
				{ "name": "currency", "type": "string" },
				{ "name": "owner", "type": "string" },
				{ "name": "probi", "type": "string" },
				{ "name": "amount", "type": "string" },
				{ "name": "fee", "type": "string" },
				{ "name": "commission", "type": "string" },
				{ "name": "fees", "type": "string" },
				{ "name": "type", "type": "string" },
				{ "name": "hash", "type": "string", "default": "" },
				{ "name": "documentId", "type": "string", "default": "" }
			]
		}`,
		latest: `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "payout",
			"doc": "This message is sent when settlement message is to be sent",
			"fields": [
				{ "name": "address", "type": "string" },
				{ "name": "settlementId", "type": "string" },
				{ "name": "publisher", "type": "string" },
				{ "name": "altcurrency", "type": "string" },
				{ "name": "currency", "type": "string" },
				{ "name": "owner", "type": "string" },
				{ "name": "probi", "type": "string" },
				{ "name": "amount", "type": "string" },
				{ "name": "fee", "type": "string" },
				{ "name": "commission", "type": "string" },
				{ "name": "fees", "type": "string" },
				{ "name": "type", "type": "string" },
				{ "name": "hash", "type": "string", "default": "" },
				{ "name": "documentId", "type": "string", "default": "" },
				{ "name": "executedAt", "type": "string", "default": "" },
				{ "name": "walletProvider", "type": "string", "default": "" }
			]
		}`,
	}
)

// New holds all info needed to create a settlement parser
func New() *avro.Handler {
	return avro.NewHandler(
		avro.TopicKeys.Settlement,
		avro.ParseCodecs(schemas),
		attemptDecodeList,
	)
}

// DecodeBatch decodes a batch of messages
func DecodeBatch(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
) (*[]models.ConvertableTransaction, error) {
	txs := []models.ConvertableTransaction{}
	for _, msg := range msgs {
		var settlement models.Settlement
		if err := avro.TryDecode(codecs, attemptDecodeList, msg, &settlement); err != nil {
			return nil, err
		}
		if settlement.ExecutedAt == "" {
			// use the time that the message was placed on the queue if none inside of msg
			settlement.ExecutedAt = msg.Time.Format(time.RFC3339)
		}
		if settlement.WalletProvider == "" {
			settlement.WalletProvider = "uphold"
		}
		txs = append(txs, models.ConvertableTransaction(&settlement))
	}
	return &txs, nil
}
