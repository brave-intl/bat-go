package promotion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/cbr"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
)

// CredentialBinding includes info needed to redeem a single credential
type CredentialBinding struct {
	PublicKey     string `json:"publicKey" valid:"base64"`
	TokenPreimage string `json:"t" valid:"base64"`
	Signature     string `json:"signature" valid:"base64"`
}

// Suggestion encapsulates information from the user about where /how they want to contribute
type Suggestion struct {
	Type    string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel string `json:"channel"`
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
	Suggestion
	TotalAmount decimal.Decimal `json:"totalAmount"`
	Funding     []FundingSource `json:"funding"`
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

	event := SuggestionEvent{Suggestion: suggestion, TotalAmount: total, Funding: []FundingSource{}}

	for _, v := range fundingSources {
		event.Funding = append(event.Funding, v)
	}

	// FIXME enqueue in job drain table
	go service.RedeemAndCreateSuggestionEvent(ctx, requestCredentials, suggestionText, event)
	return nil
}

// RedeemAndCreateSuggestionEvent after validating that all the credential bindings
func (service *Service) RedeemAndCreateSuggestionEvent(ctx context.Context, credentials []cbr.CredentialRedemption, suggestionText string, suggestion SuggestionEvent) {
	err := service.cbClient.RedeemCredentials(ctx, credentials, suggestionText)
	if err != nil {
		// FIXME
		fmt.Println(err)
	}

	service.eventChannel <- suggestion
	// TODO emit event
}
