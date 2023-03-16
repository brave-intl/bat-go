package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	submitPrefix  = "authorize-"

	// headers
	hostHeader          = "Host"
	digestHeader        = "Digest"
	contentLengthHeader = "Content-Length"
	contentTypeHeader   = "Content-type"
	signatureHeader     = "Signature"
)

var (
	payout        = strconv.FormatInt(time.Now().Unix(), 10)
	prepareStream = preparePrefix + payout
	submitStream  = preparePrefix + payout

	prepareConfigStream = preparePrefix + "-configure"
	submitConfigStream  = submitPrefix + "-configure"
)

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env   string
	redis *redis.ClusterClient
}

func newRedisClient(ctx context.Context, env, addrs, pass, username string) (*redisClient, error) {
	tlsConfig := &tls.Config{
		ServerName: "redis",
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
	err := rc.redis.Ping(ctx).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}
	return rc, nil
}

// ConfigureWorker implements settlement client
func (rc *redisClient) ConfigureWorker(ctx context.Context, stream string, config *WorkerConfig) error {
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
		_, err = pipe.XAdd(
			ctx, &redis.XAddArgs{
				Stream: prepareStream,
				Values: map[string]interface{}{
					"data": message}},
		).Result()
		if err != nil {
			return fmt.Errorf("failed to prepare transaction: %w", err)
		}
	}
	return nil
}

// SubmitTransactions implements settlement client
func (rc *redisClient) SubmitTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, at ...*AttestedTx) error {
	pipe := rc.redis.Pipeline()

	for _, v := range at {
		body, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal attested transaction body: %w", err)
		}
		// create a request
		req, err := http.NewRequest("POST", rc.env+"/authorize", bytes.NewBuffer(body))
		if err != nil {
			return fmt.Errorf("failed to create request to sign: %w", err)
		}
		// we will be signing, need all these headers for it to go through
		req.Header.Set(hostHeader, rc.env)
		req.Header.Set(digestHeader, fmt.Sprintf("%x", sha256.Sum256(body)))
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
				hostHeader:          req.Header.Get(hostHeader),
				digestHeader:        req.Header.Get(digestHeader),
				signatureHeader:     req.Header.Get(signatureHeader),
				contentLengthHeader: req.Header.Get(contentLengthHeader),
				contentTypeHeader:   req.Header.Get(contentTypeHeader),
			},
			Body: string(body),
		}

		_, err = pipe.XAdd(
			ctx, &redis.XAddArgs{
				Stream: submitStream,
				Values: map[string]interface{}{
					"data": message}},
		).Result()
		if err != nil {
			return fmt.Errorf("failed to submit transaction: %w", err)
		}
	}
	return nil
}

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *WorkerConfig) error
	PrepareTransactions(context.Context, ...*PrepareTx) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...*AttestedTx) error
}

// NewSettlementClient instanciates a new SettlementClient for use by tooling
func NewSettlementClient(ctx context.Context, env string, config map[string]string) (SettlementClient, error) {
	return newRedisClient(ctx, env, config["addrs"], config["pass"], config["username"])
}
