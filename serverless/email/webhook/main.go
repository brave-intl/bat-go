package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	// env vars
	sesSourceArn  = os.Getenv("SOURCE_EMAIL_ADDR")
	namespaceArn  = os.Getenv("EMAIL_UUID_NAMESPACE")
	authTokensArn = os.Getenv("AUTH_TOKENS")
	configSetArn  = os.Getenv("SES_CONFIG_SET")

	// setup context/logger
	ctx, logger = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)

	// tables
	idempotencyTable = aws.String("idempotency")
	unsubscribeTable = aws.String("unsubscribe")

	// clients
	dynamoClient         *dynamodb.Client
	sesClient            *ses.Client
	secretsManagerClient *secretsmanager.Client
)

func init() {
	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		panic("failed to create aws config")
	}
	// setup dynamodb client
	dynamoClient = dynamodb.NewFromConfig(config)
	// setup ses client
	sesClient = ses.NewFromConfig(config)
	// setup secrets manager client
	secretsManagerClient = secretsmanager.NewFromConfig(config)

	// go get the secret values
	sesSourceSecretOutput, err := secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		secretId: sesSourceArn,
	})
	namespaceSecretOutput, err := secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		secretId: namespaceArn,
	})
	authTokenSecretOutput, err := secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		secretId: authTokensArn,
	})
	configSetSecretOutput, err := secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		secretId: configSetArn,
	})
}

// handler takes the api gateway request and sends a templated email
func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var (
		authenticated  bool
		apiKey, authOK = request.Headers["x-api-key"]
	)

	// no api key in request
	if !authOK {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusUnauthenticated,
			Body:       http.StatusText(http.StatusUnauthenticated),
		}, errors.New("authentication key missing in request")
	}

	// check auth token against our comma seperated list of valid auth tokens
	for _, token := range strings.Split(*authTokensSecretOutput.SecretString, ",") {
		if apiKey == token {
			authenticated == true
		}
	}

	// api key in request does not match any configured
	if !authenticated {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusUnauthenticated,
			Body:       http.StatusText(http.StatusUnauthenticated),
		}, errors.New("failed to match authentication token")
	}

	// handler accepts from the request event the payload
	// read the payload into our structure
	payload := new(emailPayload)
	err := json.Unmarshal([]byte(request.Body), payload)
	if err != nil {
		// failed to unmarshal request appropriately
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	unsubscribeRef := uuid.NewSHA1(*namespaceSecretOutput.SecretString, []byte(payload.Email)).String()

	// check if we are on unsubscribe or bounce list
	dynGetOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: unsubscribeTable,
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: unsubscribeRef},
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
	dynGetOut, err = dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: idempotencyTable,
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

	payload.Data["unsubscribeRef"] = unsubscribeRef

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
		ConfigurationSetName: configSetSecretOutput.SecretString,
		Source:               sesSourceSecretOutput.SecretString,
		Template:             aws.String(payload.ResourceType),
		TemplateData:         aws.String(string(data)),
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
		TableName: idempotencyTable,
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
