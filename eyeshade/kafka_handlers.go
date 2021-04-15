package eyeshade

import (
	"errors"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	avrocontribution "github.com/brave-intl/bat-go/eyeshade/avro/contribution"
	avroreferral "github.com/brave-intl/bat-go/eyeshade/avro/referral"
	avrosettlement "github.com/brave-intl/bat-go/eyeshade/avro/settlement"
	avrosuggestion "github.com/brave-intl/bat-go/eyeshade/avro/suggestion"
	"github.com/brave-intl/bat-go/eyeshade/countries"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

var (
	// Handlers is a map for a topic key to point to any non standard handlers
	// all others are handled by HandlerDefault
	Handlers = map[string]func(con *MessageHandler, msgs []kafka.Message) error{
		avro.TopicKeys.Suggestion:   HandleVotes,
		avro.TopicKeys.Contribution: HandleVotes,
		avro.TopicKeys.Settlement:   HandlerInsertConvertableTransaction,
		avro.TopicKeys.Referral:     HandlerInsertReferrals,
	}
	// DecodeBatchVotes a mapping to help the batch decoder find it's topic specific decoder
	DecodeBatchVotes = map[string]avro.BatchVoteDecoder{
		avro.TopicKeys.Suggestion:   avrosuggestion.DecodeBatch,
		avro.TopicKeys.Contribution: avrocontribution.DecodeBatch,
	}
)

// HandleVotes handles vote insertions
func HandleVotes(
	con *MessageHandler,
	msgs []kafka.Message,
) error {
	votes, err := DecodeBatchVotes[con.bundle.Key()](
		con.bundle.Codecs(),
		msgs,
	)
	if err != nil {
		return err
	}
	return con.service.Datastore(false).
		InsertVotes(con.Context(), *votes)
}

// HandlerInsertConvertableTransaction is the default handler for direct to transaction use cases
func HandlerInsertConvertableTransaction(
	con *MessageHandler,
	msgs []kafka.Message,
) error {
	txs, err := avrosettlement.DecodeBatch(
		con.bundle.Codecs(),
		msgs,
	)
	if err != nil {
		return err
	}
	return con.service.InsertConvertableTransactions(
		con.Context(),
		*txs,
	)
}

// HandlerInsertReferrals is the default handler for direct to transaction use cases
func HandlerInsertReferrals(
	con *MessageHandler,
	msgs []kafka.Message,
) error {
	referrals, err := avroreferral.DecodeBatch(
		con.bundle.Codecs(),
		msgs,
	)
	if err != nil {
		return err
	}
	referrals, err = ModifyReferrals(con, *referrals)
	if err != nil {
		return err
	}
	txs := []models.ConvertableTransaction{}
	for i := range *referrals {
		txs = append(txs, &(*referrals)[i])
	}
	return con.service.InsertConvertableTransactions(
		con.Context(),
		txs,
	)
}

// ModifyReferrals holds the information needed to modify referral messages correctly
func ModifyReferrals(
	con *MessageHandler,
	referrals []models.Referral,
) (*[]models.Referral, error) {
	groups, err := con.service.Datastore(true).
		GetActiveCountryGroups(con.Context())
	if err != nil {
		return nil, err
	}
	modifiers := map[string]countries.Group{}
	currencies := []string{}
	for _, group := range *groups {
		currencies = append(currencies, group.Currency)
	}
	rates, err := con.service.Clients.Ratios.FetchRate(
		con.Context(),
		altcurrency.BAT.String(),
		currencies...,
	)
	if err != nil {
		return nil, err
	}
	for _, group := range *groups {
		modifiers[group.ID.String()] = group
	}

	modified := []models.Referral{}
	for i := range referrals {
		referral := referrals[i]
		group := modifiers[referral.CountryGroupID]
		if group.Amount.Equal(decimal.Zero) {
			return nil, errors.New("the country code was not found in the modifiers")
		}
		referral.Amount = group.Amount.Div(rates.Payload[group.Currency])
		referral.AltCurrency = altcurrency.BAT
		modified = append(modified, referral)
	}
	return &modified, nil
}
