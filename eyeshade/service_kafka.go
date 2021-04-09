package eyeshade

import (
	"context"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	"github.com/brave-intl/bat-go/eyeshade/models"
)

// KafkaProducer returns a kafka message producer
func (service *Service) KafkaProducer(topic string) BatchMessageProducer {
	keyAccessed := avro.KeyToTopic[topic]
	if len(keyAccessed) > 0 {
		topic = keyAccessed
	}
	return service.producers[topic]
}

// ProduceSettlements produces settlments onto the topic
func (service *Service) ProduceSettlements(ctx context.Context, messages []models.Settlement) error {
	encodable := []avro.KafkaMessageEncodable{}
	for _, msg := range messages {
		encodable = append(encodable, avro.KafkaMessageEncodable(&msg))
	}
	return service.KafkaProducer("settlement").
		Produce(ctx, encodable...)
}
