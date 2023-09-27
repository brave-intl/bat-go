package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	uuid "github.com/satori/go.uuid"
)

// busyGroup is the error message returned by redis when a consumer group with the same name already exists.
const busyGroup = "BUSYGROUP"

const (
	// dialTimeout timeout for dialler.
	dialTimeout = 15 * time.Second
	// readTimeout timeout for socket reads.
	readTimeout = 5 * time.Second
	// writeTimeout timeout for socket writes.
	writeTimeout = 5 * time.Second
	// maxRetries number of retries before giving up.
	maxRetries = 5
	// minRetryBackoff backoff between each retry.
	minRetryBackoff = 5 * time.Millisecond
	// maxRetryBackoff backoff between each retry.
	maxRetryBackoff = 500 * time.Millisecond
)

const (
	// ReleaseLockSuccess is the value returned when a lock has been released successfully.
	ReleaseLockSuccess = 1
)

var (
	// ErrLockValueDoesNotMatch is the error returned when the provided value does not match the value
	// stored at the given key.
	ErrLockValueDoesNotMatch = errors.New("redis: lock value does not match")

	// ErrStreamNotFound is returned when the stream does not exist.
	ErrStreamNotFound = errors.New("redis: stream not found")

	// ErrGroupNotFound is returned when the consumer group is not found.
	ErrGroupNotFound = errors.New("redis: group not found")

	// ErrNoStreamEntry is returned when no stream entry was returned by the executed command.
	ErrNoStreamEntry = errors.New("redis: no stream entry")

	// ErrKeyDoesNotExist is returned when a key does not exist.
	ErrKeyDoesNotExist = errors.New("redis: key does not exist")
)

type Client struct {
	rc *redis.Client
}

// NewClient creates a new instance of redis client
func NewClient(address string, username, password string) *Client {
	return &Client{rc: redis.NewClient(&redis.Options{
		Addr:            address,
		Username:        username,
		Password:        password,
		DialTimeout:     dialTimeout,
		ReadTimeout:     readTimeout,
		MaxRetries:      maxRetries,
		MinRetryBackoff: minRetryBackoff,
		MaxRetryBackoff: maxRetryBackoff,
		WriteTimeout:    writeTimeout,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ClientAuth:         0,
			InsecureSkipVerify: true,
		},
	})}
}

func (c *Client) XGroupCreateMKStream(ctx context.Context, stream, group, start string) error {
	_, err := c.rc.XGroupCreateMkStream(ctx, stream, group, start).Result()
	if err != nil && !strings.Contains(err.Error(), busyGroup) {
		return err
	}
	return nil
}

type XAddArgs struct {
	Stream string
	Values map[string]interface{}
}

func (c *Client) XAdd(ctx context.Context, args XAddArgs) error {
	_, err := c.rc.XAdd(ctx, &redis.XAddArgs{
		Stream: args.Stream,
		Values: args.Values,
	}).Result()
	if err != nil {
		return err
	}
	return nil
}

type XReadArgs struct {
	Stream    string
	MessageID string
	Count     int64
	Block     time.Duration
}

type XMessage struct {
	ID     string
	Values map[string]interface{}
}

func (c *Client) XRead(ctx context.Context, args XReadArgs) ([]XMessage, error) {
	xStreams, err := c.rc.XRead(ctx, &redis.XReadArgs{
		Streams: []string{args.Stream, args.MessageID},
		Count:   args.Count,
		Block:   args.Block,
	}).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	if len(xStreams) != 1 {
		return nil, ErrStreamNotFound
	}

	xMsgs := make([]XMessage, 0, args.Count)
	for _, m := range xStreams[0].Messages {
		xMsgs = append(xMsgs, XMessage{
			ID:     m.ID,
			Values: m.Values,
		})
	}

	return xMsgs, nil
}

type XReadGroupArgs struct {
	Group    string
	Consumer string
	Stream   string
	Count    int64
	Block    time.Duration
	NoAck    bool
}

// TODO TEST FOR WHEN NO STREAM ERROR

func (c *Client) XReadGroup(ctx context.Context, args *XReadGroupArgs) ([]XMessage, error) {
	xStreams, err := c.rc.XReadGroup(ctx, &redis.XReadGroupArgs{
		Streams:  []string{args.Stream, ">"},
		Group:    args.Group,
		Consumer: args.Consumer,
		Count:    args.Count,
		Block:    args.Block,
	}).Result()
	if err != nil {
		return nil, err
	}

	if len(xStreams) != 1 {
		return nil, ErrStreamNotFound
	}

	xMsgs := make([]XMessage, 0, args.Count)
	for _, m := range xStreams[0].Messages {
		xMsgs = append(xMsgs, XMessage{
			ID:     m.ID,
			Values: m.Values,
		})
	}

	return xMsgs, nil
}

func (c *Client) XAck(ctx context.Context, stream string, group string, ids ...string) error {
	_, err := c.rc.XAck(ctx, stream, group, ids...).Result()
	if err != nil {
		return err
	}
	return nil
}

type XPendingArgs struct {
	Stream   string
	Group    string
	Idle     time.Duration
	Count    int64
	Consumer string
}

type XPendingEntry struct {
	ID         string
	Consumer   string
	Idle       time.Duration
	RetryCount int64
}

func (c *Client) XPending(ctx context.Context, args *XPendingArgs) ([]XPendingEntry, error) {
	xPending, err := c.rc.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: args.Stream,
		Group:  args.Group,
		Idle:   args.Idle,
		Start:  "-",
		End:    "+",
		Count:  args.Count,
	}).Result()
	if err != nil {
		return nil, err
	}

	pending := make([]XPendingEntry, 0, args.Count)
	for _, x := range xPending {
		pending = append(pending, XPendingEntry{
			ID:         x.ID,
			Consumer:   x.Consumer,
			Idle:       x.Idle,
			RetryCount: x.RetryCount,
		})
	}

	return pending, nil
}

type XClaimArgs struct {
	Stream   string
	Group    string
	Consumer string
	MinIdle  time.Duration
	Messages []string
}

func (c *Client) XClaim(ctx context.Context, args XClaimArgs) ([]XMessage, error) {
	xMsg, err := c.rc.XClaim(ctx, &redis.XClaimArgs{
		Stream:   args.Stream,
		Group:    args.Group,
		Consumer: args.Consumer,
		MinIdle:  args.MinIdle,
		Messages: args.Messages,
	}).Result()
	if err != nil {
		return nil, err
	}

	var xMsgs []XMessage
	for _, m := range xMsg {
		xMsgs = append(xMsgs, XMessage{
			ID:     m.ID,
			Values: m.Values,
		})
	}

	return xMsgs, nil
}

type XInfoGroup struct {
	Name            string
	Consumers       int64
	Pending         int64
	LastDeliveredID string
}

// XInfoGroup returns information about a specific group and stream.
func (c *Client) XInfoGroup(ctx context.Context, stream, group string) (XInfoGroup, error) {
	xInfoGroups, err := c.rc.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return XInfoGroup{}, err
	}

	for i := range xInfoGroups {
		if strings.EqualFold(xInfoGroups[i].Name, group) {
			return XInfoGroup{
				Name:            xInfoGroups[i].Name,
				Consumers:       xInfoGroups[i].Consumers,
				Pending:         xInfoGroups[i].Pending,
				LastDeliveredID: xInfoGroups[i].LastDeliveredID,
			}, nil
		}
	}

	return XInfoGroup{}, ErrGroupNotFound
}

func (c *Client) XLen(ctx context.Context, stream string) (int64, error) {
	r, err := c.rc.XLen(ctx, stream).Result()
	if err != nil {
		return 0, err
	}
	return r, nil
}

// GetLastMessage returns the last message for the given stream.
// This can be considered an O(1) lookup.
func (c *Client) GetLastMessage(ctx context.Context, stream string) (XMessage, error) {
	lastMsg, err := c.rc.XRevRangeN(ctx, stream, "+", "-", 1).Result()
	if err != nil {
		return XMessage{}, err
	}

	if len(lastMsg) != 1 {
		return XMessage{}, ErrNoStreamEntry
	}

	return XMessage{
		ID:     lastMsg[0].ID,
		Values: lastMsg[0].Values,
	}, nil
}

type SetArgs struct {
	Key        string
	Value      interface{}
	Expiration time.Duration
}

func (c *Client) Set(ctx context.Context, args SetArgs) (string, error) {
	r, err := c.rc.Set(ctx, args.Key, args.Value, args.Expiration).Result()
	if err != nil {
		return "", err
	}
	return r, nil
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	r, err := c.rc.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrKeyDoesNotExist
		}
		return "", err
	}
	return r, nil
}

// AcquireLock acquires the lock for the given key and value. If a zero argument is supplied for the expiration
// time then the lock will not expire and will be held until released.
func (c *Client) AcquireLock(ctx context.Context, key string, value uuid.UUID, expiration time.Duration) (bool, error) {
	_, err := c.rc.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("error acquiring lock for key %s: %w", key, err)
	}
	return true, nil
}

// ReleaseLock release the lock for the given key and value and returns ReleaseLockSuccess if successful and an
// error otherwise. Release lock performs a check before releasing the lock to avoid releasing a lock held by
// another client. For example, a client may acquire the lock for a given key then take longer than the expiration
// time for the acquired lock and then release a lock which had been acquired by another client in the meantime.
// Both the key and value must match the original acquired lock otherwise a ErrLockValueDoesNotMatch is returned.
func (c *Client) ReleaseLock(ctx context.Context, key string, value uuid.UUID) (int, error) {
	k := []string{key}
	num, err := Unlock.Run(ctx, c.rc, k, value).Int()
	if err != nil {
		return 0, fmt.Errorf("error releasing lock for key %s: %w", key, err)
	}

	if num == 0 {
		return 0, ErrLockValueDoesNotMatch
	}

	if num > 1 {
		return num, fmt.Errorf("error should have released 1 lock got %d", num)
	}

	return ReleaseLockSuccess, nil
}

func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	r, err := c.rc.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, err
	}
	return r, nil
}

type MemberArgs struct {
	Score  float64
	Member interface{}
}

func (c *Client) ZAddNX(ctx context.Context, key string, args MemberArgs) (int64, error) {
	r, err := c.rc.ZAddNX(ctx, key, &redis.Z{
		Score:  args.Score,
		Member: args.Member,
	}).Result()
	if err != nil {
		return 0, err
	}
	return r, nil
}

func (c *Client) ZCard(ctx context.Context, key string) (int64, error) {
	r, err := c.rc.ZCard(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return r, nil
}

func (c *Client) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	r, err := c.rc.ZRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	return r, nil
}
