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
	encodable := []avro.KafkaMessageEncodable{}
	for _, msg := range messages {
		msg := msg
		encodable = append(encodable, &msg)
	}
	key := "settlement"
	return service.Producer(key).
		Produce(
			ctx,
			KeyToEncoder[key],
			encodable...,
		)
}
