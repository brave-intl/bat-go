package eyeshade

import (
	"github.com/brave-intl/bat-go/eyeshade/avro"
	avrocontribution "github.com/brave-intl/bat-go/eyeshade/avro/contribution"
	avroreferral "github.com/brave-intl/bat-go/eyeshade/avro/referral"
	avrosettlement "github.com/brave-intl/bat-go/eyeshade/avro/settlement"
	avrosuggestion "github.com/brave-intl/bat-go/eyeshade/avro/suggestion"
	"github.com/brave-intl/bat-go/eyeshade/models"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"github.com/segmentio/kafka-go"
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
	return con.service.Datastore().
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
	referrals, err = con.service.ModifyReferrals(referrals)
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
func (service *Service) ModifyReferrals(
	referrals *[]models.Referral,
) (*[]models.Referral, error) {
	groups, err := service.Datastore(true).
		GetActiveReferralGroups(service.Context())
	if err != nil {
		return nil, err
	}
	currencies := models.CollectCurrencies(*groups...)
	rates, err := service.Clients().Ratios().FetchRate(
		service.Context(),
		altcurrency.BAT.String(),
		currencies...,
	)
	if err != nil {
		return nil, err
	}

	return models.ReferralBackfillMany(referrals, groups, rates.Payload)
}
