package avro

import (
	"encoding/json"
	"fmt"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

// KafkaMessageEncodable encodes messages
type KafkaMessageEncodable interface {
	ToNative() map[string]interface{}
}

// TopicHandler holds all information needed for a topic
type TopicHandler interface {
	Topic() string
	ToBinary(KafkaMessageEncodable) ([]byte, error)
	DecodeBatch(msgs []kafka.Message) (*[]interface{}, error)
}

// CodecDecode - Decode using avro vote codec
func CodecDecode(
	codec *goavro.Codec,
	msg kafka.Message,
	p interface{},
) error {
	native, _, err := codec.NativeFromBinary(msg.Value)
	if err != nil {
		return errorutils.Wrap(err, "error decoding vote")
	}

	// gross
	v, err := json.Marshal(native)
	if err != nil {
		return fmt.Errorf("unable to marshal avro payload: %w", err)
	}

	err = json.Unmarshal(v, p)
	if err != nil {
		return fmt.Errorf("unable to decode decoded avro payload: %w", err)
	}

	return nil
}

// CodecEncode - Encode using avro vote codec
func CodecEncode(
	codec *goavro.Codec,
	msg kafka.Message,
	p interface{},
) error {
	_, binary, err := codec.NativeFromBinary(msg.Value)
	if err != nil {
		return errorutils.Wrap(err, "error converting from binary")
	}
	native, _, err := codec.NativeFromBinary(binary)
	if err != nil {
		return errorutils.Wrap(err, "error decoding")
	}

	// gross
	v, err := json.Marshal(native)
	if err != nil {
		return fmt.Errorf("unable to marshal avro payload: %w", err)
	}

	err = json.Unmarshal(v, p)
	if err != nil {
		return fmt.Errorf("unable to decode decoded avro payload: %w", err)
	}

	return nil
}
