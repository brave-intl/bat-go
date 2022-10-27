package kafka

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/getsentry/sentry-go"
	kafkago "github.com/segmentio/kafka-go"
)

type Handler interface {
	Handle(ctx context.Context, message kafkago.Message) error
}

// ErrorHandler defines
type ErrorHandler interface {
	Handle(ctx context.Context, message kafkago.Message, errorMessage error) error
}

// TODO: Commit many messages off cache.
// TODO: Return and flush offset is fail deadletter
// TODO: Add support multithreading handler

// Consume implements consumer loop.
func Consume(ctx context.Context, reader Consumer, handler Handler, errorHandler ErrorHandler) (bool, error) {
	logger := logging.Logger(ctx, "kafka consumer")
	logger.Info().Msg("starting consumer")

	for {
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
			message, err := reader.FetchMessage(ctx)
			if err != nil {
				return true, fmt.Errorf("error fetching message key %s partition %d offset %d: %w",
					string(message.Key), message.Partition, message.Offset, err)
			}

			err = handler.Handle(ctx, message)
			if err != nil {
				logger.Err(err).Msg("error processing message sending to dlq")
				err := errorHandler.Handle(ctx, message, err)
				if err != nil {
					logger.Err(err).
						Str("key", string(message.Key)).
						Int("partition", message.Partition).
						Int64("offset", message.Offset).
						Msg("error writing message to dlq")
					sentry.CaptureException(err)
				}
			}

			err = reader.CommitMessages(ctx, message)
			if err != nil {
				logger.Err(err).
					Str("key", string(message.Key)).
					Int("partition", message.Partition).
					Int64("offset", message.Offset).
					Msg("error committing kafka message")
				sentry.CaptureException(err)
			}
		}
	}
}
