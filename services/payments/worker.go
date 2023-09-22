package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	client "github.com/brave-intl/bat-go/libs/clients"
	"github.com/brave-intl/bat-go/libs/concurrent"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/httpsignature"
	"github.com/brave-intl/bat-go/libs/payments"
	redis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

var ConsumerSet *concurrent.Set

func init() {
	ConsumerSet = concurrent.NewSet()
}

type MessageHandler func(ctx context.Context, redisClient *redis.Client, id string, data []byte) error

func HandlePrepareMessage(ctx context.Context, redisClient *redis.Client, id string, data []byte) error {
	client, err := client.New("https://nitro-payments.bsg.brave.software", "")
	if err != nil {
		return err
	}
	resp, err := requestHandler(ctx, client, "POST", "/v1/prepare", data)
	if err != nil {
		return err
	}

	sr, err := httpsignature.EncapsulateResponse(ctx, resp)
	if err != nil {
		return err
	}

	// TODO xadd to our responses topic
	fmt.Println(sr)

	return nil
}

func requestHandler(ctx context.Context, client *client.SimpleHTTPClient, method, path string, data []byte) (*http.Response, error) {
	wrapper := payments.RequestWrapper{}
	err := json.Unmarshal(data, &wrapper)
	if err != nil {
		return nil, err
	}

	fmt.Println(string(data))

	var r http.Request
	_, err = wrapper.Request.Extract(&r)
	if err != nil {
		return nil, err
	}

	//r.Method = method
	r.URL = client.BaseURL.ResolveReference(&url.URL{
		Path: r.URL.RequestURI(),
	})

	resp, err := client.Do(ctx, &r, nil)
	if resp != nil {
		retry := resp.Header.Get("retry-after")
		if retry != "" {
			// TODO handle advised retry-after
			// maybe just have as part of the client
		}
	}
	return resp, err
}

func HandlePrepareConfigMessage(ctx context.Context, redisClient *redis.Client, id string, data []byte) error {
	return handleConfigMessage(HandlePrepareMessage, ctx, redisClient, id, data)
}

func handleConfigMessage(handle MessageHandler, ctx context.Context, redisClient *redis.Client, id string, data []byte) error {
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

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("childGroup", config.ConsumerGroup)
	})

	logger.Info().Msg("processed config")
	go func() {
		NewConsumer(consumerCtx, redisClient, config.Stream, config.ConsumerGroup, "0", handle)
	}()

wait:
	for {
		resp, err := redisClient.XInfoGroups(ctx, config.Stream).Result()
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

/*
func main() {
	ctx := context.Background()

	stream := "prepare-config"
	consumerGroup := "prepare-config-cg"

	cfg := payments.WorkerConfig{
		PayoutID:      "test",
		ConsumerGroup: "prepare-test-cg",
		Stream:        "prepare-test",
		Count:         1,
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", "127.0.0.1", "6379"),
	})
	_, err := redisClient.XAdd(
		ctx,
		&redis.XAddArgs{
			Stream: stream,
			Values: map[string]interface{}{
				"data": cfg,
			},
		},
	).Result()
	if err != nil {
		log.Fatal(err)
	}
	_, err = redisClient.XAdd(
		ctx,
		&redis.XAddArgs{
			Stream: "prepare-test",
			Values: map[string]interface{}{
				// FIXME
				"data": "{}",
			},
		},
	).Result()
	if err != nil {
		log.Fatal(err)
	}

	err = NewConsumer(ctx, redisClient, stream, consumerGroup, "0", PrepareConfigHandler)
	if err != nil {
		log.Fatal(err)
	}
}
*/

func NewConsumer(ctx context.Context, redisClient *redis.Client, stream, consumerGroup, consumerID string, handle MessageHandler) error {
	logger, err := appctx.GetLogger(ctx)

	consumerUID := stream + consumerGroup + consumerID
	if !ConsumerSet.Add(consumerUID) {
		// Another identical consumer is already running
		return nil
	}
	if err != nil {
		return err
	}

	logger.UpdateContext(func(c zerolog.Context) zerolog.Context {
		return c.Str("stream", stream).Str("consumerGroup", consumerGroup).Str("consumerID", consumerID)
	})

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

				err := handle(ctx, redisClient, messageID, []byte(sData))
				if err != nil {
					logger.Error().Err(err).Msg("Handler returned an error")
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
