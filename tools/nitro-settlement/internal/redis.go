package internal

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/schollz/progressbar/v3"
)

const (
	PrepareConfigStream   = "prepare-config"
	AuthorizeConfigStream = "authorize-config"

	prepareStreamPrefixFmt   = "prepare-%s"
	authorizeStreamPrefixFmt = "authorize-%s"

	poolSize = 1000
)

// Publisher - interface allowing publishing of a report
type Publisher interface {
	// PublishReport - perform the publishing of this report
	PublishReport(context.Context, string, *Report) (string, int, error)
	// SignAndPublishTransactions - perform the publishing of the signed transactions
	SignAndPublishTransactions(context.Context, string, *AttestedReport, string, ed25519.PrivateKey) (string, int, error)
}

// Configurer - interface allowing configuration of a workers
type Configurer interface {
	// ConfigureWorker - perform configuration of workers
	ConfigureWorker(context.Context, string, *WorkerConfig) error
}

type client struct {
	redis *redis.ClusterClient
}

// prepareWrapper - the redis stream structure for prepare workers
type prepareWrapper struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Body      string    `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (pw prepareWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(pw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// PublishReport - implement report publisher, returns stream/error, takes payoutid and report
func (c *client) PublishReport(ctx context.Context, payoutID string, report *Report) (string, int, error) {
	if c.redis == nil {
		return "", 0, LogAndError(ctx, nil, "PublishReport", "no configured publisher client")
	}
	var (
		begin = make(chan PrepareTx, poolSize) // connections in redis pool
		done  = make(chan struct{}, poolSize)  // connections in redis pool
		erred = make(chan error)
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := fmt.Sprintf(prepareStreamPrefixFmt, payoutID)
	numRecords := len(*report)

	// pipeline report items into buffered channel for processing
	go func(ctx context.Context, report *Report) {
		for i := 0; i < numRecords; i++ { // feed in the messages into the buffered channel
			select {
			case <-ctx.Done(): // context canceled
				logging.Logger(ctx, "PublishReport").Debug().Msg("told to stop")
				return
			default:
				begin <- []PrepareTx(*report)[i]
			}
		}
	}(ctx, report)

	// spin up poolSize workers
	for j := 0; j < poolSize; j++ {
		go func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done(): // context canceled
					logging.Logger(ctx, "PublishReport").Debug().Msg("told to stop")
					return
				default:
					// marshal transaction from begin pipeline
					body, err := json.Marshal(<-begin)
					if err != nil {
						erred <- err
					}

					// build message wrapper
					message := &prepareWrapper{
						ID:        uuid.New(),
						Timestamp: time.Now(),
						Body:      string(body),
					}

					// add to stream
					_, err = c.redis.XAdd(
						ctx, &redis.XAddArgs{
							Stream: stream,
							Values: map[string]interface{}{
								"data": message}},
					).Result()
					if err != nil {
						erred <- err
					}
					done <- struct{}{} // inform of completion of this task
				}
			}
		}(ctx)
	}

	bar := progressbar.Default(int64(len(*report)))

	for k := numRecords; k > 0; k-- {
		select {
		case err := <-erred:
			return "", k, LogAndError(ctx, err, "PublishReport", "failed submission of prepared transaction")
		default:
			<-done // wait until all complete successfully
			bar.Add(1)
		}
	}

	return stream, numRecords, nil
}

// authorizeWrapper - the redis stream structure for prepare workers
type authorizeWrapper struct {
	ID            uuid.UUID `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Host          string    `json:"host"`
	Digest        string    `json:"digest"`
	Signature     string    `json:"signature"`
	ContentLength string    `json:"contentLength"`
	ContentType   string    `json:"contentType"`
	Body          string    `json:"body"`
}

// MarshalBinary implements encoding.BinaryMarshaler required for go-redis
func (aw authorizeWrapper) MarshalBinary() (data []byte, err error) {
	bytes, err := json.Marshal(aw)
	if err != nil {
		return nil, fmt.Errorf("event message: error marshalling binary: %w", err)
	}
	return bytes, nil
}

// SignAndPublishTransactions - implement report publisher, returns stream/error, takes payoutid and attested report
func (c *client) SignAndPublishTransactions(ctx context.Context, payoutID string, report *AttestedReport, paymentsHost string, priv ed25519.PrivateKey) (string, int, error) {

	if c.redis == nil {
		return "", 0, LogAndError(ctx, nil, "PublishSignedTransactions", "no configured publisher client")
	}
	var (
		begin = make(chan AuthorizeTx, poolSize) // connections in redis pool
		done  = make(chan struct{})
		erred = make(chan error)
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream := fmt.Sprintf(authorizeStreamPrefixFmt, payoutID)
	numRecords := len(*report)

	// setup parameterized signator
	ps := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			Algorithm: httpsignature.ED25519,
			KeyID:     hex.EncodeToString([]byte(priv.Public().(ed25519.PublicKey))),
			Headers: []string{
				"(request-target)",
				"host",
				"digest",
				"content-length",
				"content-type",
			},
		},
		Signator: priv, // sign with priv
		Opts:     crypto.Hash(0),
	}

	// pipeline report items into buffered channel for processing
	go func(ctx context.Context, report *AttestedReport) {
		for i := 0; i < len(*report); i++ { // feed in the messages into the buffered channel
			select {
			case <-ctx.Done(): // context canceled
				logging.Logger(ctx, "SignAndPublishTransactions").Debug().Msg("told to stop")
				return
			default:
				begin <- []AuthorizeTx(*report)[i]
			}
		}
	}(ctx, report)

	// spin up poolSize workers
	for j := 0; j < poolSize; j++ {
		go func(ctx context.Context) {
			for {
				select {
				case <-ctx.Done(): // context canceled
					logging.Logger(ctx, "SignAndPublishTransactions").Debug().Msg("told to stop")
					return
				default:
					// marshal transaction
					body, err := json.Marshal(<-begin)
					if err != nil {
						erred <- err
					}
					// create a request
					req, err := http.NewRequest("POST", fmt.Sprintf("%s/authorize", paymentsHost), bytes.NewBuffer(body))
					if err != nil {
						erred <- err
					}
					// we will be signing, need all these headers for it to go through
					req.Header.Set("Host", paymentsHost)
					req.Header.Set("Digest", fmt.Sprintf("%x", sha256.Sum256([]byte{})))
					req.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
					req.Header.Set("Content-Type", "application/json")

					// http sign the request
					err = ps.SignRequest(req)
					if err != nil {
						erred <- err
					}

					// populate the authorizeWrapper for submission
					message := &authorizeWrapper{
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
						erred <- err
					}
					done <- struct{}{}
				}
			}
		}(ctx)
	}

	bar := progressbar.Default(int64(len(*report)))

	for k := numRecords; k > 0; k-- {
		select {
		case err := <-erred:
			return "", k, LogAndError(ctx, err, "SignAndPublishTransactions", "failed submission of authorized transaction")
		default:
			<-done // wait until all complete successfully
			bar.Add(1)
		}
	}

	return stream, numRecords, nil
}

// ConfigureWorker - publish config for prepare worker
func (c *client) ConfigureWorker(ctx context.Context, stream string, conf *WorkerConfig) error {
	_, err := c.redis.XAdd(
		ctx, &redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": conf}},
	).Result()
	if err != nil {
		return LogAndError(ctx, err, "ConfigureWorker", "failed to push config to workers")
	}
	return nil
}

// GetPublisher - get something capable of publishing a report somewhere
func GetPublisher(ctx context.Context, addrs []string, username, pass string) (*client, error) {
	tlsConfig := &tls.Config{
		ServerName: "redis",
		MinVersion: tls.VersionTLS12,
		ClientAuth: 0,
	}
	// --test-mode will cause tls to skip the insecure verification, --test-mode not for production use
	if testMode, ok := ctx.Value(TestModeCTXKey).(bool); ok && testMode {
		certPool := x509.NewCertPool()
		pem, err := ioutil.ReadFile("test/redis/tls/ca.crt")
		if err != nil {
			return nil, LogAndError(ctx, err, "GetPublisher", "failed to read test-mode ca.crt")
		}
		certPool.AppendCertsFromPEM(pem)
		tlsConfig.RootCAs = certPool
	}
	c := &client{
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
	err := c.redis.Ping(ctx).Err()
	if err != nil {
		return nil, LogAndError(ctx, err, "GetPublisher", "failed to ping redis")
	}
	return c, nil
}
