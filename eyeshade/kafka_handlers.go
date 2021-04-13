package eyeshade

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	avroreferral "github.com/brave-intl/bat-go/eyeshade/avro/referral"
	avrosettlement "github.com/brave-intl/bat-go/eyeshade/avro/settlement"
	avrosuggestion "github.com/brave-intl/bat-go/eyeshade/avro/suggestion"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/linkedin/goavro"
	"github.com/segmentio/kafka-go"
)

var (
	// Handlers is a map for a topic key to point to any non standard handlers
	// all others are handled by HandlerDefault
	Handlers = map[string]func(con *MessageHandler, msgs []kafka.Message) error{
		avro.KeyToTopic["suggestion"]: HandlerSuggestion,
	}
	// KeyToConvertableTransactionDecoder the default handler for turning messages into convertalbe transactions
	KeyToConvertableTransactionDecoder = map[string]func(
		codecs map[string]*goavro.Codec,
		msgs []kafka.Message,
		_ ...map[string]string,
	) (*[]models.ConvertableTransaction, error){
		"referral":   avroreferral.DecodeBatch,
		"settlement": avrosettlement.DecodeBatch,
	}
)

// HandlerSuggestion handles messages from the suggestion queue that will be inserted as votes
func HandlerSuggestion(con *MessageHandler, msgs []kafka.Message) error {
	suggestions, err := avrosuggestion.DecodeBatch(
		KeyToEncoder["suggestion"].Codecs(),
		msgs,
	)
	if err != nil {
		return err
	}
	return con.service.Datastore(false).
		InsertSuggestions(con.Context(), *suggestions)
}

// HandlerDefault is the default handler for direct to transaction use cases
func HandlerDefault(con *MessageHandler, msgs []kafka.Message) error {
	modifiers, err := con.Modifiers()
	if err != nil {
		return err
	}
	txs, err := KeyToConvertableTransactionDecoder[con.key](
		KeyToEncoder[con.key].Codecs(),
		msgs,
		modifiers...,
	)
	if err != nil {
		return err
	}
	return con.service.InsertConvertableTransactions(
		con.Context(),
		*txs,
	)
}
