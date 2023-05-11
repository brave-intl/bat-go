package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/rs/zerolog"
)

var (
	dynamoTableName = aws.String("unsubscribe")
	dynamoClient    *dynamodb.Client
)

// handler takes the api gateway request and sends a templated email
func handler(ctx context.Context) func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger := logging.Logger(ctx, "unsubscribe.handler")
	return func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		// /unsubscribe?id=<uuid>
		identifier := request.QueryStringParameters["id"]

		// uuidv5 from url
		_, err := dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: dynamoTableName,
			Item: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{Value: identifier},
			},
		})
		if err != nil {
			logger.Error().Err(err).Msg("failed to put item in dynamo")
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
}

func main() {
	// setup context/logger
	ctx, logger := logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)

	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		// failed to get the base aws config
		panic("failed to create aws config")
	}
	// setup global dynamodb client
	dynamoClient = dynamodb.NewFromConfig(config)

	lambda.Start(handler(ctx))
}
