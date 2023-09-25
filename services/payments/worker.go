package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	client "github.com/brave-intl/bat-go/libs/clients"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/payments"
	"github.com/brave-intl/bat-go/libs/redisconsumer"
	redis "github.com/redis/go-redis/v9"
)

// Worker for payments
type Worker struct {
	rc redisconsumer.StreamClient
}

// NewWorker from redis client
func NewWorker(rc *redis.Client) *Worker {
	return &Worker{rc: redisconsumer.NewStreamClient(rc)}
}

// HandlePrepareMessage by sending it to the payments service
func (w *Worker) HandlePrepareMessage(ctx context.Context, id string, data []byte) error {
	client, err := client.New("https://nitro-payments.bsg.brave.software", "")
	if err != nil {
		return err
	}
	resp, err := w.requestHandler(ctx, client, "POST", "/v1/prepare", id, data)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("response was nil")
	}
	if resp.StatusCode != http.StatusOK {
		return errors.New("response was not 200 OK")
	}

	sr, err := httpsignature.EncapsulateResponse(ctx, resp)
	if err != nil {
		return err
	}

	// TODO xadd to our responses topic
	fmt.Println(sr)

	return nil
}

// requestHandler is a generic handler for sending encapsulated http requests
func (w *Worker) requestHandler(ctx context.Context, client *client.SimpleHTTPClient, method, path string, id string, data []byte) (*http.Response, error) {
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

	wrapper := payments.RequestWrapper{}
	err = json.Unmarshal(data, &wrapper)
	if err != nil {
		return nil, err
	}

	r, err := client.NewRequest(ctx, method, path, nil, nil)
	if err != nil {
		return nil, err
	}

	_, err = wrapper.Request.Extract(r)
	if err != nil {
		return nil, err
	}

	r.URL = client.BaseURL.ResolveReference(&url.URL{
		Path: r.URL.RequestURI(),
	})

	delay := 5 * time.Second
	resp, err := client.Do(ctx, r, nil)
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
	err = w.rc.SetMessageRetryAfter(ctx, id, delay)
	if err != nil {
		logger.Error().Err(err).Msg("failed to set retry-after key")
	}

	return resp, err
}

// HandlePrepareConfigMessage creates a new prepare consumer, waiting for all messages to be consumed
func (w *Worker) HandlePrepareConfigMessage(ctx context.Context, id string, data []byte) error {
	return w.handleConfigMessage(w.HandlePrepareMessage, ctx, id, data)
}

// handleConfigMessage is a generic handler which creates a consumer, waiting for all messages to be consumed
func (w *Worker) handleConfigMessage(handle redisconsumer.MessageHandler, ctx context.Context, id string, data []byte) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	consumerCtx, cancelFunc := context.WithCancel(ctx)

	config := payments.WorkerConfig{}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	ctx, logger = logging.UpdateContext(ctx, logger.With().Str("childGroup", config.ConsumerGroup).Logger())

	logger.Info().Msg("processed config")
	go func() {
		redisconsumer.StartConsumer(consumerCtx, w.rc, config.Stream, config.ConsumerGroup, "0", handle)
	}()

wait:
	for {
		lag, pending, err := w.rc.UnacknowledgedCounts(ctx, config.Stream, config.ConsumerGroup)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get unacknowledged count")
		}
		if lag+pending == 0 {
			break wait
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
	return redisconsumer.StartConsumer(ctx, w.rc, payments.PrepareConfigStream, payments.PrepareConfigConsumerGroup, "0", w.HandlePrepareConfigMessage)
}
