package avro

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/brave-intl/bat-go/eyeshade/models"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	env = os.Getenv("ENV")
	// KeyToTopic creates a map of simple single words, to their more complex topic
	KeyToTopic = map[string]string{
		"settlement": env + ".settlement.payout",
		"suggestion": env + ".grant.suggestion",
		"vote":       env + ".payment.vote",
		"referral":   env + ".promo.referral",
	}
)

// KafkaMessageEncodable encodes messages
type KafkaMessageEncodable interface {
	ToNative() map[string]interface{}
}

// TopicHandler holds all information needed for a topic
type TopicHandler interface {
	Topic() string
	ToBinary(KafkaMessageEncodable) ([]byte, error)
	DecodeBatch(msgs []kafka.Message) (*[]models.ConvertableTransaction, error)
}

// TryDecode tries to decode the message
func TryDecode(
	codecs map[string]*goavro.Codec,
	checkMap map[string]string,
	msg kafka.Message,
	pointer interface{},
) error {
	errs := []error{}
	for partialParseKey, fullSchemaKey := range checkMap {
		hasPartial := partialParseKey != ""
		var partialParseErr error
		if hasPartial {
			partialParseErr = CodecDecode(codecs[partialParseKey], msg, pointer)
		}
		if hasPartial && partialParseErr != nil {
			errs = append(errs, partialParseErr)
		} else if fullSchemaError := CodecDecode(codecs[fullSchemaKey], msg, pointer); fullSchemaError != nil {
			errs = append(errs, fullSchemaError)
		} else {
			return nil
		}
	}
	return &errorutils.MultiError{
		Errs: errs,
	}
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
