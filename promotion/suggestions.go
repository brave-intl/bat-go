package promotion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/pkg/errors"
	"github.com/pressly/lg"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

// FIXME temporary until event producer is hooked up
var enableSuggestionJob = true

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
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
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

	createdAt, err := time.Now().UTC().MarshalText()
	if err != nil {
		return err
	}

	fundings := []map[string]interface{}{}
	for _, v := range fundingSources {
		fundings = append(fundings, map[string]interface{}{
			"type":      v.Type,
			"cohort":    v.Cohort,
			"amount":    v.Amount.String(),
			"promotion": v.PromotionID.String(),
		})
	}

	eventMap := map[string]interface{}{
		"id":          uuid.NewV4().String(),
		"createdAt":   string(createdAt),
		"channel":     suggestion.Channel,
		"type":        suggestion.Type,
		"totalAmount": total.String(),
		"funding":     fundings,
	}

	eventBinary, err := service.codecs["grant-suggestions"].BinaryFromNative(nil, eventMap)
	if err != nil {
		fmt.Println(err)
		return err
	}

	err = service.datastore.InsertSuggestion(requestCredentials, suggestionText, eventBinary)
	if err != nil {
		return err
	}

	if enableSuggestionJob {
		go func() {
			err := service.datastore.RunNextSuggestionJob(ctx, service)
			if err != nil {
				// FIXME
				log := lg.Log(ctx)
				log.Error("error processing suggestion job", err)
			}
		}()
	}

	return nil
}

// RedeemAndCreateSuggestionEvent after validating that all the credential bindings
func (service *Service) RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error {
	log := lg.Log(ctx)
	log.Info("started RedeemAndCreateSuggestionEvent")
	err := service.cbClient.RedeemCredentials(ctx, credentials, suggestionText)
	if err != nil {
		return nil
	}
	log.Info("successfully Redeem(ed)Credentials")

	// write the message
	err = service.kafkaWriter.WriteMessages(ctx,
		kafka.Message{
			Value: suggestion,
		},
	)
	if err != nil {
		return err
	}
	log.Info("wrote message without error")
	return nil
}
