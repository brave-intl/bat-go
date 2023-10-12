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
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	preparePrefix = "prepare-"
	submitPrefix  = "submit-"

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
	PrepareStream = preparePrefix + payout
	SubmitStream  = submitPrefix + payout

	PrepareConfigStream = preparePrefix + "config"
	SubmitConfigStream  = submitPrefix + "config"
)

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env   string
	redis *redis.ClusterClient
}

func newRedisClient(env, addrs, pass, username string) (*redisClient, error) {
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
		redis: redis.NewClusterClient(
			&redis.ClusterOptions{
				Addrs: strings.Split(addrs, ","), Password: pass, Username: username,
				DialTimeout:     15 * time.Second,
				WriteTimeout:    5 * time.Second,
				MaxRetries:      5,
				MinRetryBackoff: 5 * time.Millisecond,
				MaxRetryBackoff: 500 * time.Millisecond,
				PoolSize:        10,
				PoolTimeout:     30 * time.Second,
				TLSConfig:       tlsConfig,
			}),
	}
	return rc, nil
}

// ConfigureWorker implements settlement client
func (rc *redisClient) ConfigureWorker(ctx context.Context, stream string, config *WorkerConfig) error {
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to json encode config: %w", err)
	}

	cfg := &prepareWrapper{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Body:      string(body),
	}

	_, err = rc.redis.XAdd(
		ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": cfg}},
	).Result()
	if err != nil {
		return fmt.Errorf("failed to push config to workers: %w", err)
	}
	return nil
}

// PrepareTransactions implements settlement client
func (rc *redisClient) PrepareTransactions(ctx context.Context, t ...*PrepareTx) error {
	pipe := rc.redis.Pipeline()

	for _, v := range t {
		body, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to serialize transaction: %w", err)
		}

		// message wrapper for prepare
		message := &prepareWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Body:      string(body),
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
func (rc *redisClient) SubmitTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, at ...*AttestedTx) error {
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

		// populate the submitWrapper for submission
		message := &submitWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Headers: map[string]string{
				hostHeader:          req.Host,
				dateHeader:          req.Header.Get(dateHeader),
				digestHeader:        req.Header.Get(digestHeader),
				signatureHeader:     req.Header.Get(signatureHeader),
				contentLengthHeader: req.Header.Get(contentLengthHeader),
				contentTypeHeader:   req.Header.Get(contentTypeHeader),
			},
			Body: string(body),
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

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *WorkerConfig) error
	PrepareTransactions(context.Context, ...*PrepareTx) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...*AttestedTx) error
}

// NewSettlementClient instantiates a new SettlementClient for use by tooling
func NewSettlementClient(env string, config map[string]string) (SettlementClient, error) {
	return newRedisClient(env, config["addrs"], config["pass"], config["username"])
}
