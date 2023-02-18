package payments

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/go-redis/redis"
	"github.com/google/uuid"
)

const (
	preparePrefix   = "prepare-"
	authorizePrefix = "authorize-"
	poolSize        = 100
)

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env   string
	redis *redis.ClusterClient
}

func newRedisClient(env string) (*redisClient, error) {
	tlsConfig := &tls.Config{
		ServerName: "redis",
		MinVersion: tls.VersionTLS12,
		ClientAuth: 0,
	}
	if env == "local" {
		certPool := x509.NewCertPool()
		pem, err := ioutil.ReadFile("test/redis/tls/ca.crt")
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
				Addrs: addrs, Password: pass, Username: username,
				DialTimeout:     15 * time.Second,
				WriteTimeout:    5 * time.Second,
				MaxRetries:      5,
				MinRetryBackoff: 5 * time.Millisecond,
				MaxRetryBackoff: 500 * time.Millisecond,
				PoolSize:        poolSize,
				PoolTimeout:     30 * time.Second,
				TLSConfig:       tlsConfig,
			}),
	}
	err := rc.redis.Ping().Err()
	if err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}
	return rc, nil
}

// ConfigureWorker implements settlement client
func (rc *redisClient) ConfigureWorker(stream string, config *WorkerConfig) error {
	_, err := rc.redis.XAdd(
		&redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": conf}},
	).Result()
	if err != nil {
		return fmt.Errorf("failed to push config to workers: %w", error)
	}
	return nil
}

// PrepareTransaction implements settlement client
func (rc *redisClient) PrepareTransaction(t *Tx) error {
	// message wrapper for prepare
	message := &prepareWrapper{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Body:      string(body),
	}

	// add to stream
	_, err = rc.redis.XAdd(
		ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": message}},
	).Result()
	if err != nil {
		return fmt.Errorf("failed to prepare transaction: %w", err)
	}
	return nil
}

// SubmitTransaction implements settlement client
func (rc *redisClient) SubmitTransaction(signer httpsignature.ParameterizedSignator, at *AttestedTx) error {
	body, err := json.Marshal(at)
	if err != nil {
		return fmt.Errorf("failed to marshal attested transaction body: %w", err)
	}
	// create a request
	req, err := http.NewRequest("POST", rc.env+"/authorize", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request to sign: %w", err)
	}
	// we will be signing, need all these headers for it to go through
	req.Header.Set("Host", rc.env)
	req.Header.Set("Digest", fmt.Sprintf("%x", sha256.Sum256(body)))
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
	req.Header.Set("Content-Type", "application/json")

	// http sign the request
	err = signer.SignRequest(req)
	if err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	// populate the submitWrapper for submission
	message := &submitWrapper{
		ID:            uuid.New(),
		Timestamp:     time.Now(),
		Host:          req.Header.Get("Host"),
		Digest:        req.Header.Get("Digest"),
		Signature:     req.Header.Get("Signature"),
		ContentLength: req.Header.Get("Content-Length"),
		ContentType:   req.Header.Get("Content-Type"),
		Body:          string(body),
	}

	_, err = c.redis.XAdd(
		ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": message}},
	).Result()
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %w", err)
	}
	return nil
}

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(string, *WorkerConfig) error
	PrepareTransactions(string, *Tx) error
	SubmitTransaction(string, *AttestedTx) error
}

// NewSettlementClient instanciates a new SettlementClient for use by tooling
func NewSettlementClient(env string) (SettlementClient, error) {
	return newRedisClient(env)
}
