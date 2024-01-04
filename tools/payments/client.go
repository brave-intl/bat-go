package payments

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
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

type PayoutReportStatus struct {
	PrepareCount   int64
	PrepareLag     int64
	PreparePending int64
	SubmitCount    int64
	SubmitLag      int64
	SubmitPending  int64
}

// SettlementClient describes functionality of the settlement client
type SettlementClient interface {
	ConfigureWorker(context.Context, string, *payments.WorkerConfig) error
	PrepareTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.PrepareRequest) error
	SubmitTransactions(context.Context, httpsignature.ParameterizedSignator, ...payments.SubmitRequest) error
	WaitForResponses(ctx context.Context, payoutID string, numTransactions int, stream, cg string) error
	GetStatus(ctx context.Context, payoutID string) (*PayoutReportStatus, error)
}

// NewSettlementClient instantiates a new SettlementClient for use by tooling
func NewSettlementClient(ctx context.Context, env string, config map[string]string) (context.Context, SettlementClient, error) {
	ctx, _ = logging.SetupLogger(ctx)

	var sp httpsignature.SignatureParams
	sp.Algorithm = httpsignature.AWSNITRO
	sp.KeyID = "primary"
	sp.Headers = []string{"digest", "date"}

	pcr2, err := hex.DecodeString(config["pcr2"])
	if err != nil {
		return nil, nil, err
	}

	verifier := httpsignature.NewNitroVerifier(map[uint][]byte{2: []byte(pcr2)})

	client, err := newRedisClient(ctx, env, config["addr"], config["username"], config["pass"], &sp, verifier)
	return ctx, client, err
}

// redisClient is an implementation of settlement client using clustered redis client
type redisClient struct {
	env             string
	paymentsAPIBase string
	redis           *redisconsumer.RedisClient
	sp              *httpsignature.SignatureParams
	verifier        httpsignature.Verifier
}

func newRedisClient(ctx context.Context, env, addr, username, pass string, sp *httpsignature.SignatureParams, verifier httpsignature.Verifier) (*redisClient, error) {
	redis, err := redisconsumer.NewStreamClient(ctx, env, addr, username, pass, false)
	if err != nil {
		return nil, err
	}

	rc := &redisClient{
		env:             env,
		paymentsAPIBase: payments.APIBase[env],
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

		// to avoid errors enqueuing large message sets, enqueue them in chunks
		chunkSize := 10000
		for i := 0; i < len(messages); i += chunkSize {
			end := i + chunkSize
			if len(messages) < end {
				end = len(messages)
			}
			err := rc.redis.AddMessages(ctx, stream, messages[i:end]...)
			if err != nil {
				return fmt.Errorf("failed to enqueue transactions to redis: %w", err)
			}
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
	headerDate, err := time.Parse(time.RFC1123, resp.Header.Get("date"))
	if err != nil {
		return err
	}
	nitroVerifier, ok := rc.verifier.(httpsignature.NitroVerifier)
	if !ok {
		return nil
	}
	nitroVerifier.Now = func() time.Time { return headerDate }

	valid, err := rc.sp.VerifyResponse(nitroVerifier, crypto.Hash(0), &resp)
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

func (rc *redisClient) WaitForResponses(ctx context.Context, payoutID string, numTransactions int, stream, cg string) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	consumerGroup := stream + "-" + cg
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
			logger.Info().Int64("lag", lag).Int64("pending", pending).Msg("waiting for responses to be processed")
			if lag+pending == 0 {
				break wait
			}

		}
		time.Sleep(10 * time.Second)
	}
	cancelFunc()
	return nil
}

func (rc *redisClient) GetStatus(ctx context.Context, payoutID string) (*PayoutReportStatus, error) {
	status := PayoutReportStatus{}

	stream := payments.PreparePrefix + payoutID
	consumerGroup := stream + "-cg"

	count, err := rc.redis.GetStreamLength(ctx, stream)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream length: %v", err)
	}
	status.PrepareCount = count

	lag, pending, err := rc.redis.UnacknowledgedCounts(ctx, stream, consumerGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get unacknowledged count: %v", err)
	}
	status.PrepareLag = lag
	status.PreparePending = pending

	stream = payments.SubmitPrefix + payoutID
	consumerGroup = stream + "-cg"

	count, err = rc.redis.GetStreamLength(ctx, stream)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream length: %v", err)
	}
	status.SubmitCount = count

	lag, pending, err = rc.redis.UnacknowledgedCounts(ctx, stream, consumerGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get unacknowledged count: %v", err)
	}
	status.SubmitLag = lag
	status.SubmitPending = pending

	return &status, nil
}
