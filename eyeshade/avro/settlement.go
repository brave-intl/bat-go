package avro

import (
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	latest            = "v3"
	attemptDecodeList = []string{latest, "v2", "v1"}
	schemas           = map[string]string{
		// "executedAt": `{
		// 	"namespace": "brave.payments",
		// 	"type": "record",
		// 	"name": "payoutExecutedAt",
		// 	"aliases": ["payout"],
		// 	"fields": [
		// 		{ "name": "executedAt", "type": "string" }
		// 	]
		// }`,
		// "hashOnly": `{
		// 	"namespace": "brave.payments",
		// 	"type": "record",
		// 	"name": "payoutHash",
		// 	"aliases": ["payout"],
		// 	"fields": [
		// 		{ "name": "hash", "type": "string" }
		// 	]
		// }`,
		// it is important to keep this as a group that is not reported live to preserve anonymity
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

// NewSettlement holds all info needed to create a settlement parser
func NewSettlement() *Settlement {
	codecs := map[string]*goavro.Codec{}
	for key, schema := range schemas {
		codec, err := goavro.NewCodec(schema)
		if err != nil {
			panic(fmt.Sprintf("unable to parse %s %v", key, err))
		}
		codecs[key] = codec
	}
	return &Settlement{
		KeyToTopic["settlement"],
		schemas,
		codecs,
	}
}

// Settlement holds all info needed for settlements
type Settlement struct {
	topic   string
	schemas map[string]string
	codecs  map[string]*goavro.Codec
}

// Topic returns the settlement type's topic
func (s *Settlement) Topic() string {
	return s.topic
}

// ToBinary returns binary value of the encodable message
func (s *Settlement) ToBinary(encodable KafkaMessageEncodable) ([]byte, error) {
	return s.codecs[latest].BinaryFromNative(nil, encodable.ToNative())
}

// Decode decodes a message
func (s *Settlement) Decode(msg kafka.Message) (*models.Settlement, error) {
	var settlement models.Settlement
	if err := TryDecode(s.codecs, attemptDecodeList, msg, &settlement); err != nil {
		return nil, err
	}
	if settlement.ExecutedAt == "" { // use the time that the message was placed on the queue if none inside of msg
		settlement.ExecutedAt = msg.Time.Format(time.RFC3339)
	}
	if settlement.WalletProvider == "" {
		settlement.WalletProvider = "uphold"
	}
	return &settlement, nil
}

// DecodeBatch decodes a batch of messages
func (s *Settlement) DecodeBatch(msgs []kafka.Message) (*[]models.ConvertableTransaction, error) {
	txs := []models.ConvertableTransaction{}
	for _, msg := range msgs {
		result, err := s.Decode(msg)
		if err != nil {
			return nil, err
		}
		txs = append(txs, models.ConvertableTransaction(result))
	}
	return &txs, nil
}
