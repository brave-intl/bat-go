package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

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
	EventType     string         `json:"eventType"`
	Mail          *mail          `json:"mail"`
	Bounce        *bounce        `json:"bounce"`
	Complaint     *complaint     `json:"complaint"`
	Delivery      *delivery      `json:"delivery"`
	DeliveryDelay *deliveryDelay `json:"deliveryDelay"`
}

type mail struct {
	MessageID string              `json:"messageId"`
	Tags      []map[string]string `json:"tags"`
}

type bounce struct {
	Timestamp time.Time `json:"timestamp"`
}

type complaint struct {
	Timestamp time.Time `json:"timestamp"`
}

type delivery struct {
	Timestamp time.Time `json:"timestamp"`
}

type deliveryDelay struct {
	Timestamp time.Time `json:"timestamp"`
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

		// Check the status type and get the associated timestamp
		// Bounce, Complaint, Delivery, Send, Reject, Open, Click, Rendering Failure, DeliveryDelay, or Subscription.
		var statusTimestamp time.Time
		switch notification.EventType {
		case "Bounce":
			statusTimestamp = notification.Bounce.Timestamp
		case "Complaint":
			statusTimestamp = notification.Complaint.Timestamp
		case "Delivery":
			statusTimestamp = notification.Delivery.Timestamp
		case "Send":
		case "Reject":
		case "Open":
			continue // Skip "Open" events
		case "Click":
			continue // Skip "Click" events
		case "Rendering Failure":
		case "DeliveryDelay":
			statusTimestamp = notification.DeliveryDelay.Timestamp
		case "Subscription":
		default:
			logger.Warn().Msg("unknown event type " + notification.EventType)
		}

		// Get Idempotency key from tags to use as partition key, skip if it is not present
		var idempotencyKey string
		for _, tag := range notification.Mail.Tags {
			idempotencyKey, _ = tag["idempotencyKey"]
		}

		if idempotencyKey == "" {
			logger.Warn().Msg("missing idempotency ID from email " + notification.Mail.MessageID)
			continue
		}

		// Write status to database
		_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: statusTable,
			Item: map[string]types.AttributeValue{
				"FtxIdempotencyKey": &types.AttributeValueMemberS{Value: idempotencyKey},
				"SesMessageId":      &types.AttributeValueMemberS{Value: notification.Mail.MessageID},
				"StatusId":          &types.AttributeValueMemberS{Value: uuid.NewString()},
				"StatusType":        &types.AttributeValueMemberS{Value: notification.EventType},
				"StatusTs":          &types.AttributeValueMemberN{Value: strconv.FormatInt(statusTimestamp.Unix(), 10)},
				"CreatedAt":         &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().UTC().Unix(), 10)},
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