package eyeshade

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/segmentio/kafka-go"
)

// MessageHandler holds information about a single handler
type MessageHandler struct {
	handler avro.TopicHandler
	service *Service
	reader  *kafka.Reader
	writer  *kafka.Writer
	dialer  *kafka.Dialer
}

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
func (con *MessageHandler) Produce(ctx context.Context, encodables ...avro.KafkaMessageEncodable) error {
	messages := []kafka.Message{}
	if len(encodables) == 0 {
		return sql.ErrNoRows
	}
	for _, encodable := range encodables {
		bytes, err := con.handler.Encode(encodable)
		if err != nil {
			return err
		}
		messages = append(messages, kafka.Message{
			Value: bytes,
		})
	}
	if len(messages) == 0 {
		return errors.New("no messages encoded")
	}
	return con.writer.WriteMessages(
		ctx,
		messages...,
	)
}
