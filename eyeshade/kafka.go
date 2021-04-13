package eyeshade

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	stringutils "github.com/brave-intl/bat-go/utils/string"
	"github.com/segmentio/kafka-go"
)

// BatchMessagesHandler handles many messages being consumed at once
type BatchMessagesHandler interface {
	Topic() string
}

// BatchMessageConsumer holds methods for batch method consumption
type BatchMessageConsumer interface {
	Consume(erred chan error)
	Read(*kafka.Batch) ([]kafka.Message, bool, error)
	Handler([]kafka.Message) error
	Commit([]kafka.Message) error
}

// BatchMessageProducer holds methods for batch method producer
type BatchMessageProducer interface {
	Produce(context.Context, ...avro.KafkaMessageEncodable) error
}

// MessageHandler holds information about a single handler
type MessageHandler struct {
	handler avro.TopicHandler
	service *Service
	reader  *kafka.Reader
	writer  *kafka.Writer
	dialer  *kafka.Dialer
	conn    *kafka.Conn
}

// Topic returns the topic of the message handler
func (con *MessageHandler) Topic() string {
	return con.handler.Topic()
}

// Read reads messages
func (con *MessageHandler) Read(batch *kafka.Batch) ([]kafka.Message, bool, error) {
	msgs := []kafka.Message{}
	limit := 100
	timeout := time.Second * 2
	start := time.Now()
	for {
		msg, err := batch.ReadMessage()
		if err != nil {
			return msgs, false, err
		}
		msgs := append(msgs, msg)
		if len(msgs) >= limit {
			return msgs, false, nil
		}
		if time.Now().Add(timeout).After(start) {
			return msgs, true, nil
		}
	}
}

// Handler handles the batch of messages
func (con *MessageHandler) Handler(msgs []kafka.Message) error {
	txs, err := con.handler.DecodeBatch(msgs)
	if err != nil {
		return err
	}
	return con.service.InsertConvertableTransactions(
		con.Context(),
		*txs,
	)
}

// Context returns the context from the service
func (con *MessageHandler) Context() context.Context {
	return con.service.Context()
}

// Commit commits messages that have been read and inserted
func (con *MessageHandler) Commit(msgs []kafka.Message) error {
	return con.reader.CommitMessages(con.Context(), msgs...)
}

// Consume starts the consumer
func (con *MessageHandler) Consume(
	erred chan error,
) {
	var err error
	batch := con.conn.ReadBatchWith(kafka.ReadBatchConfig{
		MinBytes: 1,
		MaxBytes: 1e7,
	})
	defer batch.Close()
	for { // loop to continue consuming
		msgs, _, e := con.Read(batch)
		if e != nil {
			err = errorutils.Wrap(e, "during read")
			break
		}
		if len(msgs) == 0 {
			continue
		}
		e = con.Handler(msgs)
		if e != nil {
			err = errorutils.Wrap(e, "during handler")
			break
		}
		e = con.Commit(msgs)
		if e != nil {
			err = errorutils.Wrap(e, "during commit")
			break
		}
	}
	erred <- errorutils.Wrap(
		err,
		fmt.Sprintf("error in topic - %s", con.reader.Config().Topic),
	)
}

// Produce produces messages
func (con *MessageHandler) Produce(
	ctx context.Context,
	encodables ...avro.KafkaMessageEncodable,
) error {
	messages := []kafka.Message{}
	if len(encodables) == 0 {
		return sql.ErrNoRows
	}
	for _, encodable := range encodables {
		bytes, err := con.handler.ToBinary(encodable)
		if err != nil {
			return err
		}
		messages = append(messages, kafka.Message{
			Value: bytes,
		})
	}
	return con.writer.WriteMessages(
		ctx,
		messages...,
	)
}

// WithTopicAutoCreation creates topics used by producers and consumers
func WithTopicAutoCreation(service *Service) error {
	topics := map[string]int{}
	if service.producers != nil {
		for key := range service.producers {
			topics[key] = 0
		}
	}
	if service.consumers != nil {
		for key := range service.consumers {
			topics[key] = 0
		}
	}

	ctx := service.Context()
	kafkaBrokers := ctx.Value(appctx.KafkaBrokersCTXKey).(string)
	broker := stringutils.SplitAndTrim(kafkaBrokers)[0]
	conn, err := service.dialer.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer func() {
		err := conn.Close()
		if err != nil {
			fmt.Println("connection close failed", err)
		}
	}()
	reps, err := strconv.Atoi(os.Getenv("KAFKA_REPLICATIONS"))
	if err != nil {
		return err
	}
	partitions, err := strconv.Atoi(os.Getenv("KAFKA_PARTITIONS"))
	if err != nil {
		return err
	}
	for topic := range topics {
		err = conn.CreateTopics(kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     partitions,
			ReplicationFactor: reps,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// WithConsumer sets up a consumer on the service
func WithConsumer(
	topicHandlers ...avro.TopicHandler,
) func(*Service) error {
	return func(service *Service) error {
		if service.consumers == nil {
			// can't access consumer keys until make is called
			service.consumers = make(map[string]BatchMessageConsumer)
		}
		ctx := service.Context()
		leader, dialer, err := kafkautils.NewKafkaLeader(ctx)
		if err != nil {
			return err
		}
		service.dialer = dialer
		for _, topicHandler := range topicHandlers {
			reader, dialer, err := kafkautils.InitKafkaReader(
				service.Context(),
				topicHandler.Topic(),
				service.dialer,
			)
			if err != nil {
				return err
			}
			consumer := &MessageHandler{
				handler: topicHandler,
				reader:  reader,
				dialer:  dialer,
				conn:    leader,
				service: service,
			}
			service.consumers[topicHandler.Topic()] = BatchMessageConsumer(consumer)
		}
		return nil
	}
}

// WithProducer sets up a consumer on the service
func WithProducer(
	topicHandlers ...avro.TopicHandler,
) func(*Service) error {
	return func(service *Service) error {
		if service.producers == nil {
			// can't access producer keys until make is called
			service.producers = make(map[string]BatchMessageProducer)
		}
		for _, topicHandler := range topicHandlers {
			writer, dialer, err := kafkautils.InitKafkaWriter(
				service.Context(),
				topicHandler.Topic(),
				service.dialer,
			)
			if err != nil {
				return err
			}
			producer := &MessageHandler{
				handler: topicHandler,
				writer:  writer,
				dialer:  dialer,
				service: service,
			}
			service.dialer = dialer
			service.producers[topicHandler.Topic()] = BatchMessageProducer(producer)
		}
		return nil
	}
}
