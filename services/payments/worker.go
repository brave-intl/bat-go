package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	client "github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/concurrent"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/payments"
	redis "github.com/redis/go-redis/v9"
)

const RetryAfterPrefix = "retry-after-"

var ConsumerSet *concurrent.Set

func init() {
	ConsumerSet = concurrent.NewSet()
}

type MessageHandler func(ctx context.Context, id string, data []byte) error

type RedisService struct {
	rc *redis.Client
}

func NewRedisService(rc *redis.Client) *RedisService {
	return &RedisService{rc}
}

func (rs *RedisService) HandlePrepareMessage(ctx context.Context, id string, data []byte) error {
	client, err := client.New("https://nitro-payments.bsg.brave.software", "")
	if err != nil {
		return err
	}
	resp, err := rs.requestHandler(ctx, client, "POST", "/v1/prepare", id, data)
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

func (rs *RedisService) requestHandler(ctx context.Context, client *client.SimpleHTTPClient, method, path string, id string, data []byte) (*http.Response, error) {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return nil, err
	}

	err = rs.rc.Get(ctx, RetryAfterPrefix+id).Err()
	if err == nil {
		return nil, errors.New("waiting for retry-after")
	}
	if err != redis.Nil {
		return nil, err
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
	err = rs.rc.Set(ctx, RetryAfterPrefix+id, "", delay).Err()
	if err != nil {
		logger.Error().Err(err).Msg("failed to set retry-after key")
	}

	return resp, err
}

func (rs *RedisService) HandlePrepareConfigMessage(ctx context.Context, id string, data []byte) error {
	return rs.handleConfigMessage(rs.HandlePrepareMessage, ctx, id, data)
}

func (rs *RedisService) handleConfigMessage(handle MessageHandler, ctx context.Context, id string, data []byte) error {
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
		NewConsumer(consumerCtx, rs.rc, config.Stream, config.ConsumerGroup, "0", handle)
	}()

wait:
	for {
		resp, err := rs.rc.XInfoGroups(ctx, config.Stream).Result()
		if err != nil {
			return err
		}
		for _, group := range resp {
			if group.Name == config.ConsumerGroup {
				if group.Lag+group.Pending == 0 {
					break wait
				}
				logger.Info().Int64("lag", group.Lag).Int64("pending", group.Pending).Msg("waiting")

				time.Sleep(10 * time.Second)
			}
		}
	}

	logger.Info().Msg("all messages handled")
	cancelFunc()

	return nil
}

func NewConsumer(ctx context.Context, redisClient *redis.Client, stream, consumerGroup, consumerID string, handle MessageHandler) error {
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		return err
	}

	consumerUID := stream + consumerGroup + consumerID
	if !ConsumerSet.Add(consumerUID) {
		// Another identical consumer is already running
		return nil
	}
	if err != nil {
		return err
	}
	ctx, logger = logging.UpdateContext(ctx, logger.With().Str("stream", stream).Str("consumerGroup", consumerGroup).Str("consumerID", consumerID).Logger())

	logger.Info().Msg("consumer started")

	err = redisClient.XGroupCreateMkStream(ctx, stream, consumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		logger.Error().Err(err).Msg("XGROUP CREATE MKSTREAM failed")
		return err
	}

	readAndHandle := func(id string) {
		entries, err := redisClient.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerID,
			Streams:  []string{stream, id},
			Count:    5,
			Block:    100 * time.Millisecond,
			NoAck:    false,
		}).Result()
		if err != nil && !strings.Contains(err.Error(), "redis: nil") {
			logger.Error().Err(err).Msg("XREADGROUP failed")
		}

		if len(entries) > 0 {
			for i := 0; i < len(entries[0].Messages); i++ {
				messageID := entries[0].Messages[i].ID
				values := entries[0].Messages[i].Values
				data, exists := values["data"]
				if !exists {
					logger.Error().Msg("data did not exist in message")
				}
				sData, ok := data.(string)
				if !ok {
					logger.Error().Msg("data was not a string")
				}

				tmp := logger.With().Str("messageID", messageID).Logger()
				logger = &tmp
				ctx = logger.WithContext(ctx)

				err := handle(ctx, messageID, []byte(sData))
				if err != nil {
					if !strings.Contains(err.Error(), "retry-after") {
						logger.Warn().Err(err).Msg("message handler returned an error")
					}
				} else {
					redisClient.XAck(ctx, stream, consumerGroup, messageID)
				}
			}
		}
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			// read and handle new messages
			readAndHandle(">")

			// read and handle pending messages
			readAndHandle("0")
		case <-ctx.Done():
			logger.Info().Msg("shutting down consumer")
			return nil
		}
	}
}
