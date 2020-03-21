package promotion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	contextutil "github.com/brave-intl/bat-go/utils/context"
	raven "github.com/getsentry/raven-go"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
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
	Type        string                     `json:"type"`
	Amount      decimal.Decimal            `json:"amount"`
	Cohort      string                     `json:"cohort"`
	PromotionID uuid.UUID                  `json:"promotion"`
	Credentials []cbr.CredentialRedemption `json:"-"`
}

// SuggestionEvent encapsulates user and server provided information about a request to contribute
type SuggestionEvent struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Suggestion
	TotalAmount decimal.Decimal `json:"totalAmount"`
	Funding     []FundingSource `json:"funding"`
}

// TryUpgradeSuggestionEvent from JSON format to Avro, filling in any potentially missing fields
func (service *Service) TryUpgradeSuggestionEvent(suggestion []byte) ([]byte, error) {
	var event SuggestionEvent

	if suggestion[0] == '{' {
		// Assume we have a legacy JSON event
		err := json.Unmarshal(suggestion, &event)
		if err != nil {
			return []byte{}, err
		}

		if event.CreatedAt.IsZero() {
			event.CreatedAt = time.Now().UTC()
		}

		eventJSON, err := json.Marshal(event)
		if err != nil {
			return []byte{}, err
		}

		native, _, err := service.codecs["suggestion"].NativeFromTextual(eventJSON)
		if err != nil {
			return []byte{}, err
		}

		binary, err := service.codecs["suggestion"].BinaryFromNative(nil, native)
		if err != nil {
			return []byte{}, err
		}
		return binary, nil
	}
	return suggestion, nil
}

// SuggestionWorker attempts to work on a suggestion job by redeeming the credentials and emitting the event
type SuggestionWorker interface {
	RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error
}

// GetCredentialRedemptions as well as total and funding sources from a list of credential bindings
func (service *Service) GetCredentialRedemptions(ctx context.Context, credentials []CredentialBinding) (total decimal.Decimal, requestCredentials []cbr.CredentialRedemption, fundingSources map[string]FundingSource, promotions map[string]*Promotion, err error) {
	total = decimal.Zero
	requestCredentials = make([]cbr.CredentialRedemption, len(credentials))
	fundingSources = make(map[string]FundingSource)
	promotions = make(map[string]*Promotion)
	err = nil

	issuers := make(map[string]*Issuer)

	for i := 0; i < len(credentials); i++ {
		var ok bool
		var issuer *Issuer
		var promotion *Promotion

		publicKey := credentials[i].PublicKey

		if issuer, ok = issuers[publicKey]; !ok {
			issuer, err = service.datastore.GetIssuerByPublicKey(publicKey)
			if err != nil {
				err = errors.Wrap(err, "Error finding issuer")
				return
			}
		}

		requestCredentials[i].Issuer = issuer.Name()
		requestCredentials[i].TokenPreimage = credentials[i].TokenPreimage
		requestCredentials[i].Signature = credentials[i].Signature

		if promotion, ok = promotions[publicKey]; !ok {
			promotion, err = service.datastore.GetPromotion(issuer.PromotionID)
			if err != nil {
				err = errors.Wrap(err, "Error finding promotion")
				return
			}
		}
		value := promotion.CredentialValue()
		total = total.Add(value)

		fundingSource, ok := fundingSources[publicKey]
		fundingSource.Amount = fundingSource.Amount.Add(value)
		fundingSource.Credentials = append(fundingSource.Credentials, requestCredentials[i])
		if !ok {
			fundingSource.Type = promotion.Type
			fundingSource.Cohort = "control"
			fundingSource.PromotionID = promotion.ID
		}
		fundingSources[publicKey] = fundingSource
	}
	return
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

	createdAt, err := time.Now().UTC().MarshalText()
	if err != nil {
		return err
	}

	total, requestCredentials, fundingSources, _, err := service.GetCredentialRedemptions(ctx, credentials)
	if err != nil {
		return err
	}

	fundings := []map[string]interface{}{}
	metrics := map[string]decimal.Decimal{}
	fundingTypes := []string{}
	for _, v := range fundingSources {
		val, existed := metrics[v.Type]
		if !existed {
			fundingTypes = append(fundingTypes, v.Type)
			val = decimal.Zero
		}
		metrics[v.Type] = val.Add(v.Amount)
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

	eventBinary, err := service.codecs["suggestion"].BinaryFromNative(nil, eventMap)
	if err != nil {
		return err
	}

	err = service.datastore.InsertSuggestion(requestCredentials, suggestionText, eventBinary)
	if err != nil {
		return err
	}

	for _, fundingType := range fundingTypes {
		total := metrics[fundingType]
		value, _ := total.Float64()
		labels := prometheus.Labels{
			"type":    suggestion.Type,
			"funding": fundingType,
		}
		countContributionsTotal.With(labels).Inc()
		countContributionsBatTotal.With(labels).Add(value)
	}

	if enableSuggestionJob {
		asyncCtx, asyncCancel := context.WithTimeout(context.Background(), time.Minute)
		ctx = contextutil.Wrap(ctx, asyncCtx)
		go func() {
			defer asyncCancel()
			_, err := service.datastore.RunNextSuggestionJob(ctx, service)
			if err != nil {
				log.Ctx(ctx).
					Error().
					Err(err).
					Msg("error processing suggestion job")
				raven.CaptureMessage("error processing suggestion job", nil)
			}
		}()
	}

	return nil
}

// RedeemAndCreateSuggestionEvent after validating that all the credential bindings
func (service *Service) RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion []byte) error {
	suggestion, err := service.TryUpgradeSuggestionEvent(suggestion)
	if err != nil {
		return err
	}

	err = service.cbClient.RedeemCredentials(ctx, credentials, suggestionText)
	if err != nil {
		return err
	}

	// write the message
	err = service.kafkaWriter.WriteMessages(ctx,
		kafka.Message{
			Value: suggestion,
		},
	)
	if err != nil {
		return err
	}
	return nil
}
