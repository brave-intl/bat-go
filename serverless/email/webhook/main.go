package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var (
	// env vars
	sesSource              = os.Getenv("SOURCE_EMAIL_ADDR")
	namespaceArn           = os.Getenv("EMAIL_UUID_NAMESPACE")
	authTokensArn          = os.Getenv("AUTH_TOKENS")
	configSet              = os.Getenv("SES_CONFIG_SET")
	dynamoRoleArn          = os.Getenv("DYNAMODB_ROLE_ARN")
	dynamoEndpoint         = os.Getenv("DYNAMODB_ENDPOINT")
	sesDomainSourceArn     = os.Getenv("SES_DOMAIN_SOURCE_ARN")
	sesDomainFromArn       = os.Getenv("SES_DOMAIN_FROM_ARN")
	sesDomainReturnPathArn = os.Getenv("SES_DOMAIN_RETURN_PATH_ARN")

	// setup context/logger
	ctx, logger = logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)

	// tables
	idempotencyTable = aws.String("idempotency")
	unsubscribeTable = aws.String("unsubscribe")

	// clients
	dynamoClient         *dynamodb.Client
	sesClient            *ses.Client
	secretsManagerClient *secretsmanager.Client

	namespaceSecretOutput *secretsmanager.GetSecretValueOutput
	authTokenSecretOutput *secretsmanager.GetSecretValueOutput
)

func init() {
	logger.Info().Msg("init lambda")

	// setup base aws config
	config, err := appaws.BaseAWSConfig(ctx, logger)
	if err != nil {
		panic("failed to create aws config")
	}
	// setup ses client
	sesClient = ses.NewFromConfig(config)
	// setup secrets manager client
	secretsManagerClient = secretsmanager.NewFromConfig(config)

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
	logMode := aws.LogRequestWithBody | aws.LogResponseWithBody | aws.LogRetries

	// sts assume creds
	stsClient := sts.NewFromConfig(config)
	creds := stscreds.NewAssumeRoleProvider(stsClient, dynamoRoleArn)
	dynConfig.Credentials = creds
	dynConfig.ClientLogMode = logMode

	// setup dynamodb client
	dynamoClient = dynamodb.NewFromConfig(dynConfig)

	logger.Info().Msg("aws clients setup")

	namespaceSecretOutput, err = secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(namespaceArn),
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get namespace from secrets manager")
		panic("failed to get namespace cannot start service")
	}
	authTokenSecretOutput, err = secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(authTokensArn),
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth token from secrets manager")
		panic("failed to get auth tokens cannot start service")
	}
	logger.Info().Msg("secrets retreived")
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
			StatusCode: http.StatusUnauthorized,
			Body:       http.StatusText(http.StatusUnauthorized),
		}, nil
	}

	// check auth token against our comma seperated list of valid auth tokens
	for _, token := range strings.Split(*authTokenSecretOutput.SecretString, ",") {
		if apiKey == token {
			authenticated = true
		}
	}

	// api key in request does not match any configured
	if !authenticated {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusUnauthorized,
			Body:       http.StatusText(http.StatusUnauthorized),
		}, nil
	}

	logger.Info().Msg("passes auth check")

	// handler accepts from the request event the payload
	// read the payload into our structure
	payload := new(emailPayload)
	err := json.Unmarshal([]byte(request.Body), payload)
	if err != nil {
		logger.Error().Err(err).Msg("failed to unmarshal request body")
		// failed to unmarshal request appropriately
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, nil
	}

	// perform input payload validation
	valid, err := govalidator.ValidateStruct(payload)
	if err != nil {
		logger.Error().Err(err).Msg("failed to validate the body structure")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	if !valid {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, nil
	}

	logger.Info().Msg("passes input validation")

	// parse the namespace
	namespace, err := uuid.Parse(*namespaceSecretOutput.SecretString)
	if err != nil {
		logger.Error().Err(err).Msg("failed to parse the namespace into a uuid")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	unsubscribeRef := uuid.NewSHA1(namespace, []byte(payload.Email)).String()

	logger.Info().
		Str("endpoint", dynamoEndpoint).
		Str("role", dynamoRoleArn).
		Msg("performing get item from dynamo")

	// check if we are on unsubscribe or bounce list
	dynGetOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: unsubscribeTable,
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: unsubscribeRef},
		},
		ConsistentRead: aws.Bool(true), // consistent read
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get item from dynamodb")
		// failed to get the base aws config
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}
	logger.Info().Msg("got past the get item call")

	// check if it exists, if we should not send email, they unsubscribed
	if len(dynGetOut.Item) > 0 {
		logger.Error().Msg("duplicate request")
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
			"FtxIdempotencyKey": &types.AttributeValueMemberS{Value: payload.UUID},
		},
		ConsistentRead: aws.Bool(true), // consistent read
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get idempotency key from dynamodb")
		// failed to get the base aws config
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	// check if it exists, if so we already processed
	if len(dynGetOut.Item) > 0 {
		// return ok
		logger.Info().Msg("already processed this request")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusOK,
			Body:       http.StatusText(http.StatusOK),
		}, nil
	}

	if payload.Data == nil {
		payload.Data = map[string]interface{}{}
	}

	payload.Data["unsubscribeRef"] = unsubscribeRef

	// marshal template data into json
	data, err := json.Marshal(payload.Data)
	if err != nil {
		logger.Error().Err(err).Msg("failed to marshal data from payload")
		// failed to unmarshal request appropriately
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       http.StatusText(http.StatusBadRequest),
		}, nil
	}

	// send email get ses message id
	sesOut, err := sesClient.SendTemplatedEmail(ctx, &ses.SendTemplatedEmailInput{
		Destination: &sestypes.Destination{
			ToAddresses: []string{
				payload.Email,
			}},
		Source:               aws.String(sesSource),
		SourceArn:            aws.String(sesDomainSourceArn),
		Template:             aws.String(payload.SesTemplateFromResourceType()),
		TemplateData:         aws.String(string(data)),
		Tags: []sestypes.MessageTag{
			{
				Name:  aws.String("idempotencyKey"),
				Value: aws.String(payload.UUID),
			},
		},
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to send templated email")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	// set the message id
	messageID := *sesOut.MessageId

	// uuid from payload will be the client idempotency key used with dynamo
	_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: idempotencyTable,
		Item: map[string]types.AttributeValue{
			"FtxIdempotencyKey": &types.AttributeValueMemberS{Value: payload.UUID},
			"SesMessageId":      &types.AttributeValueMemberS{Value: messageID},
		},
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to put idempotency key in dynamo")
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       http.StatusText(http.StatusInternalServerError),
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Body:       http.StatusText(http.StatusOK),
	}, nil
}

func main() {
	lambda.Start(handler)
}
