package eyeshade

import (
	"context"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
)

// Producer returns a kafka message producer
func (service *Service) Producer(key string) BatchMessageProducer {
	return service.producers[key]
}

// ProduceSettlements produces settlments onto the topic
func (service *Service) ProduceSettlements(
	ctx context.Context,
	messages []models.Settlement,
) error {
	encodable := []interface{}{}
	for _, msg := range messages {
		encodable = append(encodable, msg)
	}
	return service.Producer(avro.TopicKeys.Settlement).
		Produce(
			ctx,
			KeyToEncoder[avro.TopicKeys.Settlement],
			encodable...,
		)
}

// ProduceReferrals produces referrals onto the topic
func (service *Service) ProduceReferrals(
	ctx context.Context,
	messages []models.Referral,
) error {
	encodable := []interface{}{}
	for _, msg := range messages {
		encodable = append(encodable, msg)
	}
	return service.Producer(avro.TopicKeys.Referral).
		Produce(
			ctx,
			KeyToEncoder[avro.TopicKeys.Referral],
			encodable...,
		)
}

// ProduceSuggestions produces suggestions onto the topic
func (service *Service) ProduceSuggestions(
	ctx context.Context,
	messages []models.Suggestion,
) error {
	encodable := []interface{}{}
	for _, msg := range messages {
		encodable = append(encodable, msg)
	}
	return service.Producer(avro.TopicKeys.Suggestion).
		Produce(
			ctx,
			KeyToEncoder[avro.TopicKeys.Suggestion],
			encodable...,
		)
}

// ProduceContributions produces contributions onto the topic
func (service *Service) ProduceContributions(
	ctx context.Context,
	messages []models.Contribution,
) error {
	encodable := []interface{}{}
	for _, msg := range messages {
		encodable = append(encodable, msg)
	}
	return service.Producer(avro.TopicKeys.Contribution).
		Produce(
			ctx,
			KeyToEncoder[avro.TopicKeys.Contribution],
			encodable...,
		)
}
