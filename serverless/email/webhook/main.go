package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	sesSource          = os.Getenv("SOURCE_EMAIL_ADDR")
	namespaceArn       = os.Getenv("EMAIL_UUID_NAMESPACE")
	authTokensArn      = os.Getenv("AUTH_TOKENS")
	authSecretsArn     = os.Getenv("AUTH_SECRETS")
	configSet          = os.Getenv("SES_CONFIG_SET")
	dynamoRoleArn      = os.Getenv("DYNAMODB_ROLE_ARN")
	dynamoEndpoint     = os.Getenv("DYNAMODB_ENDPOINT")
	sesDomainSourceArn = os.Getenv("SES_DOMAIN_SOURCE_ARN")

	// tables
	idempotencyTable = aws.String("idempotency")
	unsubscribeTable = aws.String("unsubscribe")

	// clients
	dynamoClient         *dynamodb.Client
	sesClient            *ses.Client
	secretsManagerClient *secretsmanager.Client

	namespaceSecretOutput   *secretsmanager.GetSecretValueOutput
	authTokenSecretOutput   *secretsmanager.GetSecretValueOutput
	authSecretsSecretOutput *secretsmanager.GetSecretValueOutput

	// header names
	tsHeaderName  = os.Getenv("TS_HEADER_NAME")
	sigHeaderName = os.Getenv("SIG_HEADER_NAME")
	keyHeaderName = os.Getenv("KEY_HEADER_NAME")
)

// validateSignature - validate the signature provided from partner
func validateSignature(ctx context.Context, request events.APIGatewayProxyRequest) error {
	logger := logging.Logger(ctx, "webhook.validateSignature")

	// signingString => {ts}{method}/sbx/{path}{body}
	signingString := fmt.Sprintf("%s%s%s%s",
		request.Headers[tsHeaderName], request.HTTPMethod, fmt.Sprintf("/sbx%s", request.Path), request.Body)

	logger.Debug().
		Str("signingString", signingString).
		Str("tsHeaderName", tsHeaderName).
		Str("sigHeaderName", sigHeaderName).
		Str("keyHeaderName", keyHeaderName).
		Msg("validateSignature details")

	signatureBytes, err := hex.DecodeString(request.Headers[sigHeaderName])
	if err != nil {
		return fmt.Errorf("failed to decode signature from headers: %w", err)
	}

	logger.Debug().
		Str("signature", request.Headers[sigHeaderName]).
		Msg("the signature")

	var valid bool
	// sha256 hmac(apiSecret, signingString)
	for _, apiSecret := range strings.Split(*authSecretsSecretOutput.SecretString, ",") {
		mac := hmac.New(sha256.New, []byte(apiSecret))
		mac.Write([]byte(signingString))
		expectedMAC := mac.Sum(nil)

		logger.Debug().
			Str("signature", request.Headers[sigHeaderName]).
			Str("generated", hex.EncodeToString(expectedMAC)).
			Msg("the signature")

		if hmac.Equal(signatureBytes, expectedMAC) {
			valid = true
			break
		}
	}

	if !valid {
		// invalid signature
		logger.Warn().
			Msg("signature is invalid for the request")
		return errors.New("invalid signature for webhook")
	}
	return nil
}

// normalizeHeaders - convert header keys to normalized version
func normalizeHeaders(a map[string]string) map[string]string {
	var b = map[string]string{}
	for k, v := range a {
		b[strings.ToUpper(k)] = v
	}
	return b
}

// handler takes the api gateway request and sends a templated email
func handler(ctx context.Context) func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	logger := logging.Logger(ctx, "webhook.handler")
	return func(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		request.Headers = normalizeHeaders(request.Headers)
		var (
			authenticated  bool
			apiKey, authOK = request.Headers[keyHeaderName]
			apiKeyHash     = sha256.Sum256([]byte(apiKey))
		)

		logger.Info().Msgf("full request: %+v", request)

		// no api key in request
		if !authOK {
			logger.Warn().Msg("no api key in headers")
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusUnauthorized,
				Body:       http.StatusText(http.StatusUnauthorized),
			}, nil
		}

		// check auth token against our comma seperated list of valid auth tokens
		for _, token := range strings.Split(*authTokenSecretOutput.SecretString, ",") {
			tokenHash := sha256.Sum256([]byte(token))
			if subtle.ConstantTimeCompare(apiKeyHash[:], tokenHash[:]) == 1 {
				authenticated = true
			}
		}

		// api key in request does not match any configured
		if !authenticated {
			logger.Warn().Msg("no token matches configuration")
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusUnauthorized,
				Body:       http.StatusText(http.StatusUnauthorized),
			}, nil
		}

		// validate the request signature
		if err := validateSignature(ctx, request); err != nil {
			return events.APIGatewayProxyResponse{
				StatusCode: http.StatusUnauthorized,
				Body:       http.StatusText(http.StatusUnauthorized),
			}, nil
		}

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
			ConfigurationSetName: aws.String(configSet),
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
}

func main() {

	// setup ctx and logger for application
	ctx, logger := logging.SetupLoggerWithLevel(context.Background(), zerolog.InfoLevel)
	logger.Info().Msg("initializing lambda")

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

	// sts assume creds
	stsClient := sts.NewFromConfig(config)
	creds := stscreds.NewAssumeRoleProvider(stsClient, dynamoRoleArn)
	dynConfig.Credentials = creds

	// setup dynamodb client
	dynamoClient = dynamodb.NewFromConfig(dynConfig)

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

	authSecretsSecretOutput, err = secretsManagerClient.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(authSecretsArn),
	})
	if err != nil {
		logger.Error().Err(err).Msg("failed to get auth secrets from secrets manager")
		panic("failed to get auth secrets cannot start service")
	}

	if len(strings.Split(*authSecretsSecretOutput.SecretString, ",")) > 2 {
		logger.Error().Msg("there should be no more than two secret strings at a time configured")
		panic("misconfigured auth secrets, can only configure two at a time, cannot start service")
	}

	// start the lambda handler
	lambda.Start(handler(ctx))
}
