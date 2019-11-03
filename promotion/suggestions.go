package promotion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/brave-intl/bat-go/utils/closers"
	"github.com/linkedin/goavro"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

// FIXME temporary until event producer is hooked up
var enableSuggestionJob = false

// CredentialBinding includes info needed to redeem a single credential
type CredentialBinding struct {
	PublicKey     string `json:"publicKey" valid:"base64"`
	TokenPreimage string `json:"t" valid:"base64"`
	Signature     string `json:"signature" valid:"base64"`
}

// Suggestion encapsulates information from the user about where /how they want to contribute
type Suggestion struct {
	Type    string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel string `json:"channel" valid:"-"`
}

// Base64Decode unmarshalls the suggestion from a string.
func (s *Suggestion) Base64Decode(text string) error {
	var bytes []byte
	bytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, s)
	return err
}

/*
{
  "type": "auto-contribute",
  "channel": "coinmarketcap.com",
  "totalAmount": "15.0",
  "funding": [
    {
      "type": "ugp",
      "amount": "15.0",
      "cohort": "control",
      "promotion": "{{promotionId}}"
    }
  ]
}
*/

// FundingSource describes where funds for this suggestion should come from
type FundingSource struct {
	Type        string          `json:"type"`
	Amount      decimal.Decimal `json:"amount"`
	Cohort      string          `json:"cohort"`
	PromotionID uuid.UUID       `json:"promotion"`
}

// SuggestionEvent encapsulates user and server provided information about a request to contribute
type SuggestionEvent struct {
	ID uuid.UUID `json:"id"`
	Suggestion
	TotalAmount decimal.Decimal `json:"totalAmount"`
	Funding     []FundingSource `json:"funding"`
}

// SuggestionWorker attempts to work on a suggestion job by redeeming the credentials and emitting the event
type SuggestionWorker interface {
	RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error
}

// Suggest that a contribution is made
func (service *Service) Suggest(ctx context.Context, credentials []CredentialBinding, suggestionText string) error {
	var suggestion Suggestion
	err := suggestion.Base64Decode(suggestionText)
	if err != nil {
		return errors.Wrap(err, "Error decoding suggestion")
	}

	_, err = govalidator.ValidateStruct(suggestion)
	if err != nil {
		return err
	}

	total := decimal.Zero
	requestCredentials := make([]cbr.CredentialRedemption, len(credentials))
	issuers := make(map[string]*Issuer)
	promotions := make(map[string]*Promotion)
	fundingSources := make(map[string]FundingSource)

	for i := 0; i < len(credentials); i++ {
		var ok bool
		var issuer *Issuer
		var promotion *Promotion

		publicKey := credentials[i].PublicKey

		if issuer, ok = issuers[publicKey]; !ok {
			issuer, err = service.datastore.GetIssuerByPublicKey(publicKey)
			if err != nil {
				return errors.Wrap(err, "Error finding issuer")
			}
		}

		requestCredentials[i].Issuer = issuer.Name()
		requestCredentials[i].TokenPreimage = credentials[i].TokenPreimage
		requestCredentials[i].Signature = credentials[i].Signature

		if promotion, ok = promotions[publicKey]; !ok {
			promotion, err = service.datastore.GetPromotion(issuer.PromotionID)
			if err != nil {
				return errors.Wrap(err, "Error finding promotion")
			}
		}
		value := promotion.CredentialValue()
		total = total.Add(value)

		fundingSource, ok := fundingSources[publicKey]
		fundingSource.Amount = fundingSource.Amount.Add(value)
		if !ok {
			fundingSource.Type = promotion.Type
			fundingSource.Cohort = "control"
			fundingSource.PromotionID = promotion.ID
		}
		fundingSources[publicKey] = fundingSource
	}

	event := SuggestionEvent{ID: uuid.NewV4(), Suggestion: suggestion, TotalAmount: total, Funding: []FundingSource{}}

	for _, v := range fundingSources {
		event.Funding = append(event.Funding, v)
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	err = service.datastore.InsertSuggestion(requestCredentials, suggestionText, eventJSON)
	if err != nil {
		return err
	}

	if enableSuggestionJob {
		go func() {
			err := service.datastore.RunNextSuggestionJob(ctx, service)
			if err != nil {
				// FIXME
				logger := log.Ctx(ctx)
				logger.Error().Err(err).Msg("error processing suggestion job")
			}
		}()
	}

	return nil
}

// RedeemAndCreateSuggestionEvent after validating that all the credential bindings
func (service *Service) RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error {
	err := service.cbClient.RedeemCredentials(ctx, credentials, suggestionText)
	if err != nil {
		return nil
	}

	// kafka event producer below - this could probably put into a generic module
	kafkaBrokers := os.Getenv("KAFKA_BROKERS_STRING")
	kafkaWriter := kafka.NewWriter(kafka.WriterConfig{
		// by default we are waitng for acks from all nodes
		Brokers:  []string{kafkaBrokers},
		Topic:    "suggestion",
		Balancer: &kafka.LeastBytes{},
	})
	defer closers.Panic(kafkaWriter)

	// possible that this schema could be tightened more
	// also, are all fields required?
	// we re-read this file every time which is decidedly not fantastic
	schema, err := ioutil.ReadFile("../schema-registry/grant/suggestion.avsc")
	if err != nil {
		return err
	}

	codec, err := goavro.NewCodec(string(schema))
	if err != nil {
		return err
	}

	// not thrilled with double encoding here; alternative is to find a nice way to turn this into a map
	// and use codec.BinaryFromNative.
	jsonMsg, err := json.Marshal(suggestion)
	if err != nil {
		return err
	}
	textual := []byte(jsonMsg)

	// above generated into native
	native, _, err := codec.NativeFromTextual(textual)
	if err != nil {
		return err
	}

	// get the avro binary
	binary, err := codec.BinaryFromNative(nil, native)
	if err != nil {
		return err
	}

	// write the message
	err = kafkaWriter.WriteMessages(ctx,
		kafka.Message{
			Value: []byte(binary),
		},
	)
	if err != nil {
		return err
	}
	service.eventChannel <- suggestion
	return nil
}
