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
		"settlement":   env + ".settlement.payout",
		"suggestion":   env + ".grant.suggestion",
		"contribution": env + ".payment.vote",
		"referral":     env + ".promo.referral",
	}
	// TopicKeys holds human readable keys for topics
	TopicKeys = keys{
		Settlement:   "settlement",
		Contribution: "contribution",
		Referral:     "referral",
		Suggestion:   "suggestion",
	}
)

type keys struct {
	Settlement   string
	Contribution string
	Referral     string
	Suggestion   string
}

// KafkaMessageEncodable encodes messages
type KafkaMessageEncodable interface {
	ToNative() map[string]interface{}
}

// TopicBundle holds all information needed for a topic
type TopicBundle interface {
	Topic() string
	Key() string
	ToBinary(KafkaMessageEncodable) ([]byte, error)
	Codecs() map[string]*goavro.Codec
	SchemaList() []string
	ManyToBinary(
		encodables ...KafkaMessageEncodable,
	) (*[]kafka.Message, error)
}

// BatchVoteDecoder decodes a batch of vote objects
type BatchVoteDecoder func(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
) (*[]models.Vote, error)

// BatchConvertableTransactionDecoder decodes a batch of convertable transactions
type BatchConvertableTransactionDecoder func(
	codecs map[string]*goavro.Codec,
	msgs []kafka.Message,
	_ ...map[string]string,
) (*[]models.ConvertableTransaction, error)

// TryDecode tries to decode the message
func TryDecode(
	codecs map[string]*goavro.Codec,
	schemaList []string,
	msg kafka.Message,
	pointer interface{},
) error {
	errs := []error{}
	for _, fullSchemaKey := range schemaList {
		if fullSchemaError := CodecDecode(codecs[fullSchemaKey], msg, pointer); fullSchemaError != nil {
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

// Handler holds information about how to parse messages
type Handler struct {
	key    string
	codecs map[string]*goavro.Codec
	list   []string
}

// NewHandler creates a new handler
func NewHandler(
	key string,
	codecs map[string]*goavro.Codec,
	list []string,
) *Handler {
	return &Handler{
		key,
		codecs,
		list,
	}
}

// ParseCodecs parses codecs for all Handler types
func ParseCodecs(schemas map[string]string) map[string]*goavro.Codec {
	codecs := map[string]*goavro.Codec{}
	for key, schema := range schemas {
		codec, err := goavro.NewCodec(schema)
		if err != nil {
			panic(fmt.Sprintf("unable to parse %s %v", key, err))
		}
		codecs[key] = codec
	}
	return codecs
}

// Key returns the type's key
func (h *Handler) Key() string {
	return h.key
}

// Topic returns the type's topic
func (h *Handler) Topic() string {
	return KeyToTopic[h.key]
}

// ToBinary returns binary value of the encodable message
func (h *Handler) ToBinary(encodable KafkaMessageEncodable) ([]byte, error) {
	return h.codecs[h.SchemaList()[0]].BinaryFromNative(nil, encodable.ToNative())
}

// ManyToBinary converts a series of kafka encodable messages to kafka.Messages
func (h *Handler) ManyToBinary(
	encodables ...KafkaMessageEncodable,
) (*[]kafka.Message, error) {
	messages := []kafka.Message{}
	for _, encodable := range encodables {
		bytes, err := h.ToBinary(encodable)
		if err != nil {
			return nil, err
		}
		messages = append(messages, kafka.Message{
			Value: bytes,
		})
	}
	return &messages, nil
}

// SchemaList get the list of schema keys in the order they should be tried
func (h *Handler) SchemaList() []string {
	return h.list
}

// Codecs returns the codecs
func (h *Handler) Codecs() map[string]*goavro.Codec {
	return h.codecs
}
