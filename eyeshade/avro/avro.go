package avro

import (
	"encoding/json"
	"fmt"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

// TopicHandler holds all information needed for a topic
type TopicHandler interface {
	Topic() string
	Schema() string
	Codec() *goavro.Codec
	DecodeBatch(msgs []kafka.Message) (*[]interface{}, error)
}

// CodecDecode - Decode using avro vote codec
func CodecDecode(
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
