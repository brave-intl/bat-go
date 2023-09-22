package payments

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
)

const (
	// headers
	hostHeader   = "Host"
	digestHeader = "Digest"
	// dateHeader needs to be lowercase to pass the signing verifier validation.
	dateHeader          = "date"
	contentLengthHeader = "Content-Length"
	contentTypeHeader   = "Content-Type"
	signatureHeader     = "Signature"
)

var (
	payout        = strconv.FormatInt(time.Now().Unix(), 10)
	PrepareStream = payments.PreparePrefix + payout
	SubmitStream  = payments.SubmitPrefix + payout
)

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *payments.WorkerConfig) error
	PrepareTransactions(context.Context, httpsignature.ParameterizedSignator, ...*payments.PrepareTx) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...*payments.AttestedTx) error
}

// NewSettlementClient instantiates a new SettlementClient for use by tooling
func NewSettlementClient(env string, config map[string]string) (SettlementClient, error) {
	return newRedisClient(env, config["addr"], config["pass"], config["username"])
}

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env   string
	redis *redis.Client
}

func newRedisClient(env, addr, pass, username string) (*redisClient, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientAuth: 0,
	}

	// only if environment is local do we hardcode these values
	if env == "local" {
		certPool := x509.NewCertPool()
		pem, err := ioutil.ReadFile("redistest/test/redis/tls/ca.crt")
		if err != nil {
			return nil, fmt.Errorf("failed to read test-mode ca.crt: %w", err)
		}
		certPool.AppendCertsFromPEM(pem)
		tlsConfig.RootCAs = certPool
	}

	rc := &redisClient{
		env: env,
		redis: redis.NewClient(
			&redis.Options{
				Addr: addr, Password: pass, Username: username,
				DialTimeout:     15 * time.Second,
				WriteTimeout:    5 * time.Second,
				MaxRetries:      5,
				MinRetryBackoff: 5 * time.Millisecond,
				MaxRetryBackoff: 500 * time.Millisecond,
			}),
	}
	return rc, nil
}

// ConfigureWorker implements settlement client
func (rc *redisClient) ConfigureWorker(ctx context.Context, stream string, config *payments.WorkerConfig) error {
	_, err := rc.redis.XAdd(
		ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": config}},
	).Result()
	if err != nil {
		return fmt.Errorf("failed to push config to workers: %w", err)
	}
	return nil
}

// PrepareTransactions implements settlement client
func (rc *redisClient) PrepareTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, t ...*payments.PrepareTx) error {
	pipe := rc.redis.Pipeline()

	for _, v := range t {
		buf := bytes.NewBuffer([]byte{})
		err := json.NewEncoder(buf).Encode(v)
		body := buf.Bytes()

		req, err := http.NewRequest(http.MethodPost, rc.env+"/v1/payments/prepare", buf)
		if err != nil {
			return fmt.Errorf("failed to create request to sign: %w", err)
		}
		req.Header.Set(dateHeader, time.Now().Format(time.RFC1123))
		req.Header.Set(contentLengthHeader, fmt.Sprintf("%d", len(body)))
		req.Header.Set(contentTypeHeader, "application/json")

		// http sign the request
		err = signer.SignRequest(req)
		if err != nil {
			return fmt.Errorf("failed to sign request: %w", err)
		}

		er, err := httpsignature.EncapsulateRequest(req)
		if err != nil {
			return fmt.Errorf("failed to encapsulate request: %w", err)
		}

		// message wrapper for prepare
		message := &payments.RequestWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Request:   er,
		}

		// add to stream
		pipe.XAdd(
			ctx, &redis.XAddArgs{
				Stream: PrepareStream,
				Values: map[string]interface{}{
					"data": message}},
		)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to exec prepare transaction commands: %w", err)
	}

	return nil
}

// SubmitTransactions implements settlement client
func (rc *redisClient) SubmitTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, at ...*payments.AttestedTx) error {
	pipe := rc.redis.Pipeline()

	for _, v := range at {

		buf := bytes.NewBuffer([]byte{})
		err := json.NewEncoder(buf).Encode(v)
		body := buf.Bytes()
		if err != nil {
			return fmt.Errorf("failed to marshal attested transaction body: %w", err)
		}

		// Create a request and set the headers we require for signing. The Digest header is added
		// during the signing call and the request.Host is set during the new request creation so,
		// we don't need to explicitly set them here.
		req, err := http.NewRequest(http.MethodPost, rc.env+"/v1/payments/submit", buf)
		if err != nil {
			return fmt.Errorf("failed to create request to sign: %w", err)
		}
		req.Header.Set(dateHeader, time.Now().Format(time.RFC1123))
		req.Header.Set(contentLengthHeader, fmt.Sprintf("%d", len(body)))
		req.Header.Set(contentTypeHeader, "application/json")

		// http sign the request
		err = signer.SignRequest(req)
		if err != nil {
			return fmt.Errorf("failed to sign request: %w", err)
		}

		er, err := httpsignature.EncapsulateRequest(req)
		if err != nil {
			return fmt.Errorf("failed to encapsulate request: %w", err)
		}

		// populate the SubmitWrapper for submission
		message := &payments.RequestWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Request:   er,
		}

		pipe.XAdd(
			ctx, &redis.XAddArgs{
				Stream: SubmitStream,
				Values: map[string]interface{}{
					"data": message}},
		)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to exec submit transaction commands: %w", err)
	}

	return nil
}
