package avro

import (
	"os"

	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

// NewSettlement holds all info needed to create a settlement parser
func NewSettlement(
	handler func([]models.ConvertableTransaction) error,
) *Settlement {
	topic := os.Getenv("ENV") + ".settlement.payout"
	schema := `{}`
	codec, err := goavro.NewCodec(schema)
	if err != nil {
		panic("failed to parse settlement schema")
	}
	return &Settlement{
		topic,
		schema,
		codec,
		handler,
	}
}

// Settlement holds all info needed for settlements
type Settlement struct {
	topic   string
	schema  string
	codec   *goavro.Codec
	handler func([]models.ConvertableTransaction) error
}

// Topic returns the settlement type's topic
func (s *Settlement) Topic() string {
	return s.topic
}

// Codec returns the settlement type's codec
func (s *Settlement) Codec() *goavro.Codec {
	return s.codec
}

// Schema returns the settlemnet type's schema
func (s *Settlement) Schema() string {
	return s.schema
}

// Decode decodes a message
func (s *Settlement) Decode(msg kafka.Message) (*models.Settlement, error) {
	var settlement models.Settlement
	err := CodecDecode(s.Codec(), msg, &settlement)
	if err != nil {
		return nil, err
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
