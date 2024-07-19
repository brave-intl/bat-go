package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	client "github.com/brave-intl/bat-go/libs/clients"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/redisconsumer"
	"github.com/google/uuid"
)

// Worker for payments
type Worker struct {
	rc redisconsumer.StreamClient
}

// NewWorker from redis client
func NewWorker(rc redisconsumer.StreamClient) *Worker {
	return &Worker{rc}
}

// HandlePrepareMessage by sending it to the payments service
func (w *Worker) HandlePrepareMessage(ctx context.Context, stream, id string, data []byte) error {
	baseURI := os.Getenv("NITRO_API_BASE")

	client, err := client.NewWithHTTPClient(baseURI, "", &http.Client{
		Timeout: time.Second * 60,
	})
	if err != nil {
		return err
	}
	_, err = w.requestHandler(ctx, client, "POST", "/v1/prepare", stream, id, data)
	return err
}

// HandleSubmitMessage by sending it to the payments service
func (w *Worker) HandleSubmitMessage(ctx context.Context, stream, id string, data []byte) error {
	logger, err := appctx.GetLogger(ctx)
	baseURI := os.Getenv("NITRO_API_BASE")

	client, err := client.NewWithHTTPClient(baseURI, "", &http.Client{
		Timeout: time.Second * 60,
	})
	if err != nil {
		return err
	}
	sr, err := w.requestHandler(ctx, client, "POST", "/v1/submit", stream, id, data)
	if err != nil {
		return err
	}
	// requestHandler will only succeed if status is 200, in which case data is populated
	type submitResponse struct {
		Data payments.SubmitResponse `json:"data"`
	}
	// NOTE: we are not verifying the http signature
	resp := submitResponse{}
	err = json.Unmarshal([]byte(sr.Body), &resp)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to introspect submit response")
		return nil
	}

	err = w.rc.IncrementSetScore(ctx, stream+payments.StatusSuffix, string(resp.Data.Status))
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to increment settlement status count")
	}
	return nil
}

// requestHandler is a generic handler for sending encapsulated http requests and storing the results
func (w *Worker) requestHandler(ctx context.Context, client *client.SimpleHTTPClient, method, path string, stream, id string, data []byte) (*httpsignature.HTTPSignedResponse, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return nil, err
	}

	isRetryBlocked, err := w.rc.GetMessageRetryAfter(ctx, id)
	if err != nil {
		return nil, err
	}
	if isRetryBlocked {
		return nil, errors.New("waiting for retry-after")
	}

	delay := 1 * time.Second
	resp, err := httpDoWhileRetryZero(ctx, client, data, method, path)
	if resp != nil {
		retry := resp.Header.Get("x-retry-after")
		if retry != "" {
			tmp, err := strconv.Atoi(retry)
			if err != nil {
				logger.Error().Err(err).Msg("failed to parse x-retry-after header")
			}
			delay = time.Duration(tmp) * time.Second
		}
	}

	if err := w.rc.SetMessageRetryAfter(ctx, id, delay); err != nil {
		logger.Error().Err(err).Msg("failed to set retry-after key")
	}

	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("response was nil")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("response was not 200 OK")
	}

	sr, err := httpsignature.EncapsulateResponse(ctx, resp)
	if err != nil {
		return nil, err
	}

	respWrapper := &payments.ResponseWrapper{
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Response:  sr,
	}

	err = w.rc.AddMessages(ctx, stream+payments.ResponseSuffix, respWrapper)
	return sr, err
}

func httpDoWhileRetryZero(
	ctx context.Context,
	client *client.SimpleHTTPClient,
	data []byte,
	method,
	path string,
) (*http.Response, error) {
	for i := 0; i < 500; i++ {
		// Generate wrapped request. It must be generated each loop because the request and response
		// can be Closed, breaking them across iterations.
		reqWrapper := payments.RequestWrapper{}
		err := json.Unmarshal(data, &reqWrapper)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal request into wrapper: %w", err)
		}

		req, err := client.NewRequest(ctx, method, path, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create new request: %w", err)
		}

		_, err = reqWrapper.Request.Extract(req)
		if err != nil {
			return nil, fmt.Errorf("failed to extract request from wrapper: %w", err)
		}

		// FIXME we should probably complete override the url based on params
		req.URL = client.BaseURL.ResolveReference(&url.URL{
			Path: req.URL.RequestURI(),
		})
		resp, err := client.Do(ctx, req, nil)
		if resp != nil {
			retry := resp.Header.Get("x-retry-after")
			if resp.StatusCode != http.StatusOK && retry == "0" {
				continue
			}
		}
		return resp, err
	}
	return nil, nil
}

// HandlePrepareConfigMessage creates a new prepare consumer, waiting for all messages to be consumed
func (w *Worker) HandlePrepareConfigMessage(ctx context.Context, stream, id string, data []byte) error {
	return w.handleConfigMessage(ctx, w.HandlePrepareMessage, id, data)
}

// HandleSubmitConfigMessage creates a new submit consumer, waiting for all messages to be consumed
func (w *Worker) HandleSubmitConfigMessage(ctx context.Context, stream, id string, data []byte) error {
	return w.handleConfigMessage(ctx, w.HandleSubmitMessage, id, data)
}

// handleConfigMessage is a generic handler which creates a consumer, waiting for all messages to be consumed
func (w *Worker) handleConfigMessage(ctx context.Context, handle redisconsumer.MessageHandler, id string, data []byte) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	consumerCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	config := payments.WorkerConfig{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	if config.BatchSize == 0 {
		config.BatchSize = 10
	}

	ctx, logger = logging.UpdateContext(ctx, logger.With().Str("childGroup", config.ConsumerGroup).Logger())

	logger.Info().Msg("processed config")
	go func() {
		redisconsumer.StartConsumer(consumerCtx, w.rc, config.Stream, config.ConsumerGroup, "0", handle, config.BatchSize)
	}()

	for {
		lag, pending, err := w.rc.UnacknowledgedCounts(ctx, config.Stream, config.ConsumerGroup)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get unacknowledged count")
		}
		if lag+pending == 0 {
			break
		}
		logger.Info().Int64("lag", lag).Int64("pending", pending).Msg("waiting")

		time.Sleep(10 * time.Second)
	}

	logger.Info().Msg("all messages handled")
	cancelFunc()

	return nil
}

// StartPrepareConfigConsumer is a convenience function for starting the prepare config consumer
func (w *Worker) StartPrepareConfigConsumer(ctx context.Context) error {
	return redisconsumer.StartConsumer(ctx, w.rc, payments.PrepareConfigStream, payments.PrepareConfigConsumerGroup, "0", w.HandlePrepareConfigMessage, 1)
}

// StartSubmitConfigConsumer is a convenience function for starting the prepare config consumer
func (w *Worker) StartSubmitConfigConsumer(ctx context.Context) error {
	return redisconsumer.StartConsumer(ctx, w.rc, payments.SubmitConfigStream, payments.SubmitConfigConsumerGroup, "0", w.HandleSubmitConfigMessage, 1)
}
