package eyeshade

import (
	"context"
	"database/sql"
	"fmt"
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
	Read() (*[]kafka.Message, bool, error)
	Handler(*[]kafka.Message) error
	Commit(*[]kafka.Message) error
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
}

// Topic returns the topic of the message handler
func (con *MessageHandler) Topic() string {
	return con.handler.Topic()
}

// Collect collects messages from the reader
func (con *MessageHandler) Collect(
	msgCh chan<- kafka.Message,
	errCh chan<- error,
) {
	msg, err := con.reader.FetchMessage(con.Context())
	if err != nil {
		errCh <- err
	} else {
		msgCh <- msg
	}
}

// Read reads messages
func (con *MessageHandler) Read() (*[]kafka.Message, bool, error) {
	// this method is dangerous
	// need to check kafka implementation details
	msgs := []kafka.Message{}
	msgCh := make(chan kafka.Message)
	errCh := make(chan error)
	timeoutDuration := time.Second
	finished := false
	for {
		if con.reader.Config().QueueCapacity == len(msgs) {
			finished = true
			return &msgs, finished, nil
		}
		go con.Collect(msgCh, errCh)
		select {
		case err := <-errCh:
			if len(msgs) > 0 {
				// set back to beginning of batch
				offsetErr := con.reader.SetOffset(msgs[0].Offset)
				if offsetErr != nil {
					mErr := errorutils.MultiError{
						Errs: []error{err, offsetErr},
					}
					err = error(&mErr)
				}
			}
			finished = true
			return nil, finished, fmt.Errorf("could not read message %w", err)
		case msg := <-msgCh:
			if finished {
				err := con.reader.SetOffset(msg.Offset)
				if err != nil {
					return nil, finished, err
				}
			}
			msgs = append(msgs, msg)
		case <-time.After(timeoutDuration):
			finished = true
			return &msgs, finished, nil
		}
	}
}

// Handler handles the batch of messages
func (con *MessageHandler) Handler(msgs *[]kafka.Message) error {
	if msgs == nil {
		return nil
	}
	txs, err := con.handler.DecodeBatch(*msgs)
	if err != nil {
		return err
	}
	_, err = con.service.InsertConvertableTransactions(
		con.Context(),
		txs,
	)
	return err
}

// Context returns the context from the service
func (con *MessageHandler) Context() context.Context {
	return con.service.Context()
}

// Commit commits messages that have been read and inserted
func (con *MessageHandler) Commit(msgs *[]kafka.Message) error {
	if msgs == nil {
		return nil
	}
	return con.reader.CommitMessages(con.Context(), *msgs...)
}

// Consume starts the consumer
func (con *MessageHandler) Consume(
	erred chan error,
) {
	for { // loop to continue consuming
		msgs, _, err := con.Read()
		if err == nil {
			err = con.Handler(msgs)
			if err == nil {
				err = con.Commit(msgs)
			}
		}
		if err != nil {
			erred <- errorutils.Wrap(
				err,
				fmt.Sprintf("error in topic - %s", con.reader.Config().Topic),
			)
			break
		}
	}
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
	fmt.Printf("writing %d messages\n", len(messages))
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
	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		return err
	}
	defer conn.Close()
	for topic := range topics {
		err = conn.CreateTopics(kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     2,
			ReplicationFactor: 3,
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
				service: service,
			}
			service.dialer = dialer
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
