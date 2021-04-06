package eyeshade

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	stringutils "github.com/brave-intl/bat-go/utils/string"
	"github.com/segmentio/kafka-go"
)

var (
	kafkaBrokers = os.Getenv("KAFKA_BROKERS")
	env          = os.Getenv("ENV")
	groupID      = fmt.Sprintf(
		"%s.%s",
		env,
		os.Getenv("SERVICE"),
	)
)

// Consumer holds information about a single consumer
type Consumer struct {
	topicHandler avro.TopicHandler
	ctx          context.Context
	service      *Service
	reader       *kafka.Reader
	config       kafka.ReaderConfig
}

// BatchMessagesConsumer handles many messages being consumed at once
type BatchMessagesConsumer interface {
	Consume(erred chan error)
	Read() (*[]kafka.Message, bool, error)
	Handler(*[]kafka.Message) error
	Commit(*[]kafka.Message) error
}

// Collect collects messages from the reader
func (con *Consumer) Collect(
	msgCh chan<- kafka.Message,
	errCh chan<- error,
) {
	msg, err := con.reader.FetchMessage(con.ctx)
	if err != nil {
		errCh <- err
	} else {
		msgCh <- msg
	}
}

// Read reads messages
func (con *Consumer) Read() (*[]kafka.Message, bool, error) {
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

// NewKafkaReader creates a new kafka reader for a given topic
func (service *Service) NewKafkaReader(topic string) (
	*kafka.Reader,
	kafka.ReaderConfig,
) {
	brokers := stringutils.SplitAndTrim(kafkaBrokers, ",")
	config := kafka.ReaderConfig{
		MaxBytes: 1e6,
		MaxWait:  time.Second,
		Brokers:  brokers,
		Topic:    topic,
		GroupID:  groupID,
	}
	return kafka.NewReader(config), config
}

// Handler handles the batch of messages
func (con *Consumer) Handler(msgs *[]kafka.Message) error {
	if msgs == nil {
		return nil
	}
	txs, err := con.topicHandler.DecodeBatch(*msgs)
	if err != nil {
		return err
	}
	_, err = con.service.InsertConvertableTransactions(
		*con.service.ctx,
		txs,
	)
	return err
}

// Commit commits messages that have been read and inserted
func (con *Consumer) Commit(msgs *[]kafka.Message) error {
	if msgs == nil {
		return nil
	}
	return con.reader.CommitMessages(con.ctx, *msgs...)
}

// Consume starts the consumer
func (con *Consumer) Consume(
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
