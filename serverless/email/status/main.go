package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	// env vars
	dynamoRoleArn  = os.Getenv("DYNAMODB_ROLE_ARN")
	dynamoEndpoint = os.Getenv("DYNAMODB_ENDPOINT")

	// setup context/logger
	ctx, logger = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)

	// tables
	statusTable = aws.String("status")

	// clients
	dynamoClient *dynamodb.Client
)

func init() {
	// setup ctx and logger for application
	logger.Info().Msg("initializing status lambda")

	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		panic("failed to create aws config")
	}

	customResolver := aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
		if service == dynamodb.ServiceID && region == "us-west-2" {
			return aws.Endpoint{
				PartitionID:   "aws",
				URL:           fmt.Sprintf("https://%s", dynamoEndpoint),
				SigningRegion: "us-west-2",
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	dynConfig, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		panic("failed to create aws dynamo config")
	}
	dynConfig.EndpointResolver = customResolver

	// sts assume creds
	stsClient := sts.NewFromConfig(config)
	creds := stscreds.NewAssumeRoleProvider(stsClient, dynamoRoleArn)
	dynConfig.Credentials = creds

	// setup dynamodb client
	dynamoClient = dynamodb.NewFromConfig(dynConfig)
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
	Tags      map[string][]string `json:"tags"`
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
			logger.Warn().Msgf("unknown event type %s", notification.EventType)
		}

		// Get Idempotency key from tags to use as partition key, skip if it is not present
		idempotencyKeyList, found := notification.Mail.Tags["idempotencyKey"]
		if !found {
			logger.Warn().Msgf("missing idempotency ID from email %s", notification.Mail.MessageID)
			continue
		}
		if len(idempotencyKeyList) != 1 {
			logger.Warn().Msgf("missing idempotency ID from email %s", notification.Mail.MessageID)
			continue
		}
		idempotencyKey := idempotencyKeyList[0]

		// Write status to database
		item := map[string]types.AttributeValue{
			"FtxIdempotencyKey": &types.AttributeValueMemberS{Value: idempotencyKey},
			"SesMessageId":      &types.AttributeValueMemberS{Value: notification.Mail.MessageID},
			"StatusId":          &types.AttributeValueMemberS{Value: uuid.NewString()},
			"StatusType":        &types.AttributeValueMemberS{Value: notification.EventType},
			"CreatedAt":         &types.AttributeValueMemberN{Value: strconv.FormatInt(time.Now().UTC().Unix(), 10)},
		}
		// Include StatusTs only if it has a value
		if !statusTimestamp.IsZero() {
			item["StatusTs"] = &types.AttributeValueMemberN{Value: strconv.FormatInt(statusTimestamp.Unix(), 10)}
		}
		_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: statusTable,
			Item:      item,
		})
		if err != nil {
			logger.Error().Err(err).Msgf(
				"failed to write status to dynamodb for messageID %s", notification.Mail.MessageID,
			)
		}
	}
}

func main() {
	lambda.Start(handler)
}
