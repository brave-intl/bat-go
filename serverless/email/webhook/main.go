package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	ctx, logger                = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)
	dynamoTableName            = aws.String("webhook-idempotency")
	dynamoUnsubscribeTableName = aws.String("unsubscribe")
	sesSource                  = aws.String("noreply@brave.com")
	namespace                  = uuid.MustParse(os.Getenv("EMAIL_NAMESPACE"))
)

// handler takes the api gateway request and sends a templated email
func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		// failed to get the base aws config
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, fmt.Errorf("failed to get the base aws config: %w", err)
	}
	// setup dynamodb client
	dynamoClient := dynamodb.NewFromConfig(config)
	// setup ses client
	sesClient := ses.NewFromConfig(config)

	// handler accepts from the request event the payload
	// read the payload into our structure
	payload := new(emailPayload)
	err = json.Unmarshal([]byte(request.Body), payload)
	if err != nil {
		// failed to unmarshal request appropriately
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	// check if we are on unsubscribe or bounce list
	dynGetOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: dynamoUnsubscribeTableName,
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: uuid.NewSHA1(namespace, []byte(payload.UUID.String()))},
		},
		ConsistentRead: aws.Bool(true), // consistent read
	})
	if err != nil {
		// failed to get the base aws config
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, fmt.Errorf("failed to get from dynamodb: %w", err)
	}

	// check if it exists, if we should not send email, they unsubscribed
	if len(dynGetOut.Item) > 0 {
		// return ok
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Body:       http.StatusText(http.StatusOK),
		}, nil
	}

	// check if our idempotency key exists in db
	dynGetOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: dynamoTableName,
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: payload.UUID.String()},
		},
		ConsistentRead: aws.Bool(true), // consistent read
	})
	if err != nil {
		// failed to get the base aws config
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, fmt.Errorf("failed to get from dynamodb: %w", err)
	}

	// check if it exists, if so we already processed
	if len(dynGetOut.Item) > 0 {
		// return ok
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Body:       http.StatusText(http.StatusOK),
		}, nil
	}

	// marshal template data into json
	data, err := json.Marshal(payload.Data)
	if err != nil {
		// failed to unmarshal request appropriately
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, fmt.Errorf("failed to marshal ses template: %w", err)
	}

	// send email get ses message id
	sesOut, err := sesClient.SendTemplatedEmail(ctx, &ses.SendTemplatedEmailInput{
		Destination: &sestypes.Destination{
			ToAddresses: []string{
				payload.Email,
			}},
		Source:       sesSource,
		Template:     aws.String(string(payload.ResourceType)),
		TemplateData: aws.String(string(data)),
	})
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, fmt.Errorf("failed to send templated email: %w", err)
	}

	// set the message id
	messageID := *sesOut.MessageId

	// uuid from payload will be the client idempotency key used with dynamo
	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: dynamoTableName,
		Item: map[string]types.AttributeValue{
			"id":         &types.AttributeValueMemberS{Value: payload.UUID.String()},
			"message_id": &types.AttributeValueMemberS{Value: messageID},
		},
	})
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, fmt.Errorf("failed to write to dynamodb: %w", err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       http.StatusText(http.StatusOK),
	}, nil
}

func main() {
	lambda.Start(handler)
}
