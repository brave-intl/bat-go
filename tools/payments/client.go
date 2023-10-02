package payments

import (
	"bytes"
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/redisconsumer"
	"github.com/brave-intl/bat-go/libs/requestutils"
	"github.com/google/uuid"
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
	paymentsAPIBase = map[string]string{
		"":      "https://nitro-payments.bsg.brave.software",
		"local": "https://nitro-payments.bsg.brave.software",
		"dev":   "https://nitro-payments.bsg.brave.software",
	}
)

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *payments.WorkerConfig) error
	PrepareTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.PrepareRequest) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.SubmitRequest) error
	WaitForResponses(ctx context.Context, payoutID string, numTransactions int) error
}

// NewSettlementClient instantiates a new SettlementClient for use by tooling
func NewSettlementClient(ctx context.Context, env string, config map[string]string) (context.Context, SettlementClient, error) {
	ctx, _ = logging.SetupLogger(ctx)

	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{"digest"}

	// FIXME
	pcrs := map[uint][]byte{
		12: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	verifier := httpsignature.NewNitroVerifier(pcrs)

	client, err := newRedisClient(ctx, env, config["addr"], config["pass"], config["username"], sp, verifier)
	return ctx, client, err
}

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env             string
	paymentsAPIBase string
	redis           *redisconsumer.RedisClient
	sp              httpsignature.SignatureParams
	verifier        httpsignature.Verifier
}

func newRedisClient(ctx context.Context, env, addr, pass, username string, sp httpsignature.SignatureParams, verifier httpsignature.Verifier) (*redisClient, error) {
	redis, err := redisconsumer.NewStreamClient(ctx, env, addr, pass, username)
	if err != nil {
		return nil, err
	}

	rc := &redisClient{
		env:             env,
		paymentsAPIBase: paymentsAPIBase[env],
		redis:           redis,
		sp:              sp,
		verifier:        verifier,
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

		req, err := http.NewRequest(http.MethodPost, rc.paymentsAPIBase+"/v1/payments/prepare", buf)
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

	submitGroups := make(map[string][]interface{})
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
		req, err := http.NewRequest(http.MethodPost, rc.paymentsAPIBase+"/v1/payments/submit", buf)
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
		err := rc.redis.AddMessages(ctx, stream, messages...)
		if err != nil {
			return fmt.Errorf("failed to exec submit transaction commands: %w", err)
		}
	}

	return nil
}

func (rc *redisClient) HandlePrepareResponse(ctx context.Context, stream, id string, data []byte) error {
	respWrapper := payments.ResponseWrapper{}
	err := json.Unmarshal(data, &respWrapper)
	if err != nil {
		return err
	}

	resp := http.Response{}
	_, err = respWrapper.Response.Extract(&resp)
	if err != nil {
		return err
	}

	valid, err := rc.sp.VerifyResponse(rc.verifier, crypto.Hash(0), &resp)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("http signature was not valid, nitro attestation failed")
	}

	bodyBytes, err := requestutils.Read(ctx, resp.Body)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(stream+".log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(bodyBytes); err != nil {
		return err
	}

	return nil
}

func (rc *redisClient) WaitForResponses(ctx context.Context, payoutID string, numTransactions int) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	stream := payments.PreparePrefix + payoutID + payments.ResponseSuffix
	// FIXME use public key as consumer group
	consumerGroup := stream + "-cli"
	consumerID := "0"

	consumerCtx, cancelFunc := context.WithCancel(ctx)
	go func() {
		redisconsumer.StartConsumer(consumerCtx, rc.redis, stream, consumerGroup, consumerID, rc.HandlePrepareResponse)
	}()

wait:
	for {
		count, err := rc.redis.GetStreamLength(ctx, stream)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get stream length")
		}
		logger.Info().Int64("count", count).Int("total", numTransactions).Msg("waiting for responses")
		if count >= int64(numTransactions) {
			lag, pending, err := rc.redis.UnacknowledgedCounts(ctx, stream, consumerGroup)
			if err != nil {
				logger.Error().Err(err).Msg("failed to get unacknowledged count")
			}
			if lag+pending == 0 {
				break wait
			}
			logger.Info().Int64("lag", lag).Int64("pending", pending).Msg("waiting for responses to be processed")

		}
		time.Sleep(10 * time.Second)
	}
	cancelFunc()
	return nil
}
