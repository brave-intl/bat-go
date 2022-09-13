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
	ctx, logger     = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)
	dynamoTableName = aws.String("unsubscribe")
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

	// /unsubscribe?id=<uuid>
	identifier := request.QueryStringParameters["id"]

	// uuidv5 from url
	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: dynamoTableName,
		Item: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: identifier},
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
