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

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/redisconsumer"
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
	payout       = strconv.FormatInt(time.Now().Unix(), 10)
	SubmitStream = payments.SubmitPrefix + payout
)

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *payments.WorkerConfig) error
	PrepareTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.PrepareRequest) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.SubmitRequest) error
}

// NewSettlementClient instantiates a new SettlementClient for use by tooling
func NewSettlementClient(ctx context.Context, env string, config map[string]string) (context.Context, SettlementClient, error) {
	ctx, _ = logging.SetupLogger(ctx)
	client, err := newRedisClient(env, config["addr"], config["pass"], config["username"])
	return ctx, client, err
}

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env   string
	redis *redisconsumer.RedisClient
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
		redis: (*redisconsumer.RedisClient)(redis.NewClient(
			&redis.Options{
				Addr: addr, Password: pass, Username: username,
				DialTimeout:     15 * time.Second,
				WriteTimeout:    5 * time.Second,
				MaxRetries:      5,
				MinRetryBackoff: 5 * time.Millisecond,
				MaxRetryBackoff: 500 * time.Millisecond,
			})),
	}
	return rc, nil
}

// ConfigureWorker implements settlement client
func (rc *redisClient) ConfigureWorker(ctx context.Context, stream string, config *payments.WorkerConfig) error {
	err := rc.redis.AddMessages(ctx, stream, config)
	if err != nil {
		return fmt.Errorf("failed to push config to workers: %w", err)
	}
	return nil
}

// PrepareTransactions implements settlement client
func (rc *redisClient) PrepareTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, t ...payments.PrepareRequest) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	prepareGroups := make(map[string][]interface{})
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
		prepareGroups[v.PayoutID] = append(prepareGroups[v.PayoutID], payments.RequestWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Request:   er,
		})
	}

	for payoutID, messages := range prepareGroups {
		logger.Info().Str("payoutID", payoutID).Int("messages", len(messages)).Msg("prepared transactions")
		stream := payments.PreparePrefix + payoutID
		err := rc.redis.AddMessages(ctx, stream, messages...)
		if err != nil {
			return fmt.Errorf("failed to exec prepare transaction commands: %w", err)
		}
	}

	return nil
}

// SubmitTransactions implements settlement client
func (rc *redisClient) SubmitTransactions(ctx context.Context, signer httpsignature.ParameterizedSignator, at ...payments.SubmitRequest) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	submitGroups := make(map[string][]payments.RequestWrapper)
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

		if _, exists := submitGroups[v.PayoutID]; !exists {
			submitGroups[v.PayoutID] = []payments.RequestWrapper{}
		}
		// message wrapper for submit
		submitGroups[v.PayoutID] = append(submitGroups[v.PayoutID], payments.RequestWrapper{
			ID:        uuid.New(),
			Timestamp: time.Now(),
			Request:   er,
		})
	}

	for payoutID, messages := range submitGroups {
		logger.Info().Str("payoutID", payoutID).Int("messages", len(messages)).Msg("submitted transactions")
		stream := payments.SubmitPrefix + payoutID
		err := rc.redis.AddMessages(ctx, stream, messages)
		if err != nil {
			return fmt.Errorf("failed to exec submit transaction commands: %w", err)
		}
	}

	return nil
}
