package avro

import (
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/eyeshade/models"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	schemas = map[string]string{
		"hashonly": `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "payoutHash",
			"aliases": ["payout"],
			"fields": [
				{ "name": "hash", "type": "string" }
			]
		}`,
		// it is important to keep this as a group that is not reported live to preserve anonymity
		"v1": `{
			"namespace": "brave.payments",
			"type": "record",
			"name": "payout",
			"doc": "This message is sent when settlement message is to be sent",
			fields: [
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
				{ "name": "hash", "type": "string" },
				{ "name": "documentId", "type": "string" }
			]
		}`,
	}
)

// NewSettlement holds all info needed to create a settlement parser
func NewSettlement(
	handler func([]models.ConvertableTransaction) error,
) *Settlement {
	topic := os.Getenv("ENV") + ".settlement.payout"
	codecs := map[string]*goavro.Codec{}
	for key, schema := range schemas {
		codec, err := goavro.NewCodec(schema)
		if err != nil {
			panic(fmt.Sprintf("unable to parse %s", key))
		}
		codecs[key] = codec
	}
	return &Settlement{
		topic,
		schemas,
		codecs,
		handler,
	}
}

// Settlement holds all info needed for settlements
type Settlement struct {
	topic   string
	schemas map[string]string
	codecs  map[string]*goavro.Codec
	handler func([]models.ConvertableTransaction) error
}

// Topic returns the settlement type's topic
func (s *Settlement) Topic() string {
	return s.topic
}

// Decode decodes a message
func (s *Settlement) Decode(msg kafka.Message) (*models.Settlement, error) {
	var settlement models.Settlement
	err := CodecDecode(s.codecs["hashonly"], msg, &settlement)
	if err != nil {
		v1Err := CodecDecode(s.codecs["v1"], msg, &settlement)
		if v1Err == nil {
			return &settlement, nil
		}
		return nil, &errorutils.MultiError{
			Errs: []error{err, v1Err}
		}
	}
	v2Err := CodecDecode(s.codecs["v2"], msg, &settlement)
	if v2Err != nil {
		return nil, v2Err
	}
	return &settlement, nil
}

// DecodeBatch decodes a batch of messages
func (s *Settlement) DecodeBatch(msgs []kafka.Message) (*[]interface{}, error) {
	txs := []interface{}{}
	for _, msg := range msgs {
		result, err := s.Decode(msg)
		if err != nil {
			return nil, err
		}
		txs = append(txs, *result)
	}
	return &txs, nil
}
