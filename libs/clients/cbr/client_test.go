//go:build integration

package cbr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/brave-intl/bat-go/libs/clients"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/ptr"
	"github.com/brave-intl/bat-go/libs/test"
	_ "github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestCreateIssuerV1(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	err = client.CreateIssuer(ctx, "test:"+uuid.NewV4().String(), 100)
	assert.NoError(t, err, "Should be able to create issuer")
}

func TestGetIssuerV1(t *testing.T) {
	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	issuerName := "test:" + uuid.NewV4().String()

	issuer, err := client.GetIssuer(ctx, issuerName)
	assert.Error(t, err, "Should not be able to get issuer")
	// checking the error
	httpError, ok := err.(*errorutils.ErrorBundle)
	assert.Equal(t, true, ok, "should be able to coerce to an error bundle")
	httpState, ok := httpError.Data().(clients.HTTPState)
	assert.Equal(t, true, ok, "should contain an HTTPState")
	assert.Equal(t, http.StatusNotFound, httpState.Status, "status should be not found")

	err = client.CreateIssuer(ctx, issuerName, 100)
	assert.NoError(t, err, "Should be able to create issuer")

	issuer, err = client.GetIssuer(ctx, issuerName)
	assert.NoError(t, err, "Should be able to get issuer")

	assert.NotEqual(t, len(issuer.PublicKey), 0, "Should have public key")
}

func TestSignAndRedeemCredentialsV1(t *testing.T) {
	databaseURL := os.Getenv("CHALLENGE_BYPASS_DATABASE_URL")

	sKey := "fzJbqh6l/xWAjT6Ulmu+/Taxz8XZ7SDnJ/dUXPgtnQE="
	pKey := "jKj71sdk2XYMwZNSxvUfNkSNCUQeBuUxuTbdjIbupmE="
	blindedToken := "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	signedToken := "ohwnBITMSphAFK/06LtbC+PYl6PmmEhOdybvsfqZjG4="
	preimage := "Aa61pQzyxsy3Z6tSwccnOqiW23fNYp0z3xw6XGlA5FG8O/EqlxR87DWnas49U2JUau44dpiveAt7kBXDH5RjPQ=="
	sig := "zx1zdMhN4Et8WnrkVQOad6xhUBAJ7Pq4b8A0n96CRE0QdAQ+tJe0/eFiJqIPMuKkyfQ6VncIkGj9VzkByh9uFA=="
	payload := "test message"

	issuerName := "constant"

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		assert.NoError(t, err, "Must be able to connect to challenge-bypass db")
	}

	_, err = db.Exec("DELETE from v3_issuer_keys; DELETE FROM v3_issuers; DELETE from redemptions")
	assert.NoError(t, err, "Must be able to clear issuers")

	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err, "Must be able to correctly initialize the client")

	err = client.CreateIssuer(ctx, issuerName, 100)
	assert.NoError(t, err, "Should be able to create issuer")

	_, err = db.Exec("update v3_issuer_keys set signing_key=$1", sKey)
	assert.NoError(t, err, "Must be able to insert issuer key")

	issuer, err := client.GetIssuer(ctx, issuerName)
	assert.NoError(t, err, "Should be able to get issuer")
	assert.Equal(t, issuer.PublicKey, pKey, "Public key should match expected")

	resp, err := client.SignCredentials(ctx, issuerName, []string{blindedToken})
	assert.NoError(t, err, "Should be able to sign tokens")
	assert.Equal(t, resp.SignedTokens[0], signedToken, "Public key should match expected")

	err = client.RedeemCredential(ctx, issuerName, preimage, sig, payload)
	assert.NoError(t, err, "Should be able to redeem tokens")

	_, err = db.Exec("DELETE from redemptions")
	assert.NoError(t, err, "Must be able to clear redemptions")

	err = client.RedeemCredentials(ctx, []CredentialRedemption{{Issuer: issuerName, TokenPreimage: preimage, Signature: sig}}, payload)
	assert.NoError(t, err, "Should be able to bulk redeem tokens")
}

func TestCreateIssuerV3(t *testing.T) {
	client, err := New()
	assert.NoError(t, err)

	request := IssuerRequest{
		Name:      test.RandomString(),
		Cohort:    int16(test.RandomNonZeroInt(10)),
		MaxTokens: test.RandomNonZeroInt(10),
		ValidFrom: ptr.FromTime(time.Now()),
		Duration:  "P1M",
		Buffer:    test.RandomNonZeroInt(10),
		Overlap:   test.RandomNonZeroInt(10),
	}

	err = client.CreateIssuerV3(context.Background(), request)
	assert.NoError(t, err)
}

func TestSignAndRedeemCredentialsV3(t *testing.T) {
	databaseURL := os.Getenv("CHALLENGE_BYPASS_DATABASE_URL")

	sKey := "fzJbqh6l/xWAjT6Ulmu+/Taxz8XZ7SDnJ/dUXPgtnQE="
	blindedToken := "yoGo7zfMr5vAzwyyFKwoFEsUcyUlXKY75VvWLfYi7go="
	signedToken := "ohwnBITMSphAFK/06LtbC+PYl6PmmEhOdybvsfqZjG4="
	preimage := "Aa61pQzyxsy3Z6tSwccnOqiW23fNYp0z3xw6XGlA5FG8O/EqlxR87DWnas49U2JUau44dpiveAt7kBXDH5RjPQ=="
	sig := "zx1zdMhN4Et8WnrkVQOad6xhUBAJ7Pq4b8A0n96CRE0QdAQ+tJe0/eFiJqIPMuKkyfQ6VncIkGj9VzkByh9uFA=="
	payload := "test message"

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		assert.NoError(t, err, "Must be able to connect to challenge-bypass db")
	}

	_, err = db.Exec("DELETE from v3_issuer_keys; DELETE FROM v3_issuers; DELETE from redemptions")
	assert.NoError(t, err, "Must be able to clear issuers")

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	var config = &aws.Config{
		Region:   aws.String("us-west-2"),
		Endpoint: aws.String(os.Getenv("DYNAMODB_ENDPOINT")),
	}
	config.DisableSSL = aws.Bool(true)
	svc := dynamodb.New(sess, config)
	err = setupDynamodbTables(svc)
	assert.NoError(t, err)

	ctx := context.Background()

	client, err := New()
	assert.NoError(t, err)

	// If we use cohort 1 we can use the v1 SignCredentials call to mock the signing process.
	// This maybe deprecated in the future then we will need to use kafka
	issuerRequest := IssuerRequest{
		Name:      test.RandomString(),
		Cohort:    1,
		MaxTokens: test.RandomNonZeroInt(10),
		ValidFrom: ptr.FromTime(time.Now()),
		ExpiresAt: ptr.FromTime(time.Now().Add(time.Hour)),
		Duration:  "P1M",
		Buffer:    test.RandomNonZeroInt(10),
		Overlap:   test.RandomNonZeroInt(10),
	}

	err = client.CreateIssuerV3(context.Background(), issuerRequest)
	assert.NoError(t, err)

	issuer, err := client.GetIssuerV3(ctx, issuerRequest.Name)
	assert.NoError(t, err)

	assert.Equal(t, issuerRequest.Name, issuer.Name)
	assert.Equal(t, issuerRequest.Cohort, issuer.Cohort)
	assert.Equal(t, issuerRequest.ExpiresAt.Format(time.RFC3339), issuer.ExpiresAt)
	assert.NotEmpty(t, issuer.PublicKey)

	_, err = db.Exec("update v3_issuer_keys set signing_key=$1", sKey)
	assert.NoError(t, err)

	resp, err := client.SignCredentials(ctx, issuerRequest.Name, []string{blindedToken})
	assert.NoError(t, err)
	assert.Equal(t, resp.SignedTokens[0], signedToken)

	err = client.RedeemCredentialV3(ctx, issuerRequest.Name, preimage, sig, payload)
	assert.NoError(t, err)

	_, err = db.Exec("DELETE from redemptions")
	assert.NoError(t, err)
}

// setupDynamodbTables this function sets up tables for use in dynamodb tests.
func setupDynamodbTables(db *dynamodb.DynamoDB) error {
	_, _ = db.DeleteTable(&dynamodb.DeleteTableInput{
		TableName: ptr.FromString("redemptions"),
	})

	input := &dynamodb.CreateTableInput{
		TableName:   ptr.FromString("redemptions"),
		BillingMode: ptr.FromString("PAY_PER_REQUEST"),
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("id"),
				KeyType:       aws.String("HASH"),
			},
		},
	}

	_, err := db.CreateTable(input)
	if err != nil {
		return fmt.Errorf("error creating dynamodb table %w", err)
	}

	err = tableIsActive(db, *input.TableName, 5*time.Second, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("error table is not active %w", err)
	}

	return nil
}

func tableIsActive(db *dynamodb.DynamoDB, tableName string, timeout, duration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return errors.New("timed out while waiting for table status to become ACTIVE")
		case <-time.After(duration):
			table, err := db.DescribeTable(&dynamodb.DescribeTableInput{
				TableName: aws.String(tableName),
			})
			if err != nil {
				return fmt.Errorf("instance.DescribeTable error %w", err)
			}
			if table.Table == nil || table.Table.TableStatus == nil || *table.Table.TableStatus != "ACTIVE" {
				continue
			}
			return nil
		}
	}
}
