package main

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go/aws"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	// setup context/logger
	ctx, logger = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)

	// tables
	statusTable = aws.String("status")

	// clients
	dynamoClient *dynamodb.Client
)

func init() {
	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		panic("failed to create aws config")
	}
	// setup dynamodb client
	dynamoClient = dynamodb.NewFromConfig(config)
}

// We use event publishing to receive delivery status notifications via SNS
// https://docs.aws.amazon.com/ses/latest/dg/event-publishing-retrieving-sns-contents.html
type sesNotification struct {
	EventType string `json:"eventType"`
	Mail      *mail  `json:"mail"`
}

type mail struct {
	MessageID string              `json:"messageId"`
	Tags      []map[string]string `json:"tags"`
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

		// Get Idempotency ID from tags to use as partition key, skip if it is not present
		var idempotencyID string
		for _, tag := range notification.Mail.Tags {
			idempotencyID, _ = tag["idempotencyID"]
		}

		if idempotencyID == "" {
			logger.Warn().Msg("missing idempotency ID from email " + notification.Mail.MessageID)
			continue
		}

		_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: statusTable,
			Item: map[string]types.AttributeValue{
				"idempotency_id": &types.AttributeValueMemberS{Value: idempotencyID},
				"status_id":      &types.AttributeValueMemberS{Value: uuid.NewString()},
				"message_id":     &types.AttributeValueMemberS{Value: notification.Mail.MessageID},
				"type":           &types.AttributeValueMemberS{Value: notification.EventType},
			},
		})

		if err != nil {
			logger.Error().Err(err).Msg(
				"failed to write status to dynamodb for messageID " + notification.Mail.MessageID,
			)
		}
	}
}

func main() {
	lambda.Start(handler)
}
