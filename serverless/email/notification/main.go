package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/rs/zerolog"
)

var (
	ctx, logger = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)
)

// We use event publishing to receive delivery status notifications via SNS
// https://docs.aws.amazon.com/ses/latest/dg/event-publishing-retrieving-sns-contents.html
type sesNotification struct {
	EventType string `json:"eventType"`
	Mail      mail   `json:"mail"`
}

type mail struct {
	MessageID string `json:"messageId"`
}

func handler(ctx context.Context, snsEvent events.SNSEvent) {
	// Process the SES delivery notifications included in the SNS event.
	// An event may include notifications one or multiple recipients
	// A single recipient might receive multiple notifications
	// https://docs.aws.amazon.com/ses/latest/dg/notification-contents.html
	for _, record := range snsEvent.Records {
		var notification sesNotification
		err := json.Unmarshal([]byte(record.SNS.Message), &notification)
		if err != nil {
			logger.Error().Err(err).Msg("failed to unmarshal notification")
			continue
		}

		// TODO Write notificationType, timestamp to database, using messageID as key
		// TODO Should these be batch written outside of the loop?
	}
}

func main() {
	lambda.Start(handler)
}
