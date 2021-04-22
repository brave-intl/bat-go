package eyeshade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/brave-intl/bat-go/eyeshade/avro"
	avrocontribution "github.com/brave-intl/bat-go/eyeshade/avro/contribution"
	avroreferral "github.com/brave-intl/bat-go/eyeshade/avro/referral"
	avrosettlement "github.com/brave-intl/bat-go/eyeshade/avro/settlement"
	avrosuggestion "github.com/brave-intl/bat-go/eyeshade/avro/suggestion"
	appctx "github.com/brave-intl/bat-go/utils/context"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

var (
	// KeyToEncoder maps a key to an avro topic bundle
	KeyToEncoder = map[string]avro.TopicBundle{
		avro.TopicKeys.Settlement:   avrosettlement.New(),
		avro.TopicKeys.Referral:     avroreferral.New(),
		avro.TopicKeys.Suggestion:   avrosuggestion.New(),
		avro.TopicKeys.Contribution: avrocontribution.New(),
	}
)

// BatchMessagesHandler handles many messages being consumed at once
type BatchMessagesHandler interface {
	Topic() string
}

// BatchMessageConsumer holds methods for batch method consumption
type BatchMessageConsumer interface {
	AutoCreate()
	// Consume(erred chan error)
	Read(*kafka.Conn) ([]kafka.Message, error)
	HandleAndCommit(context.Context, *zerolog.Logger, Committer, []kafka.Message) error
	IdleFor(time.Duration) bool
	TopicCreated() bool
	Topic() string
	Stop()
	IsStopped() bool
}

// BatchMessageProducer holds methods for batch method producer
type BatchMessageProducer interface {
	AutoCreate()
	Produce(
		context.Context,
		avro.TopicBundle,
		...interface{},
	) error
}

// Committer commits topic offsets
type Committer interface {
	CommitOffsets(map[string]map[int]int64) error
}

// MessageHandler holds information about a single handler
type MessageHandler struct {
	key          string
	bundle       avro.TopicBundle
	service      *Service
	writer       *kafka.Writer
	dialer       *kafka.Dialer
	batchLimit   int
	isHandling   bool
	isConsuming  bool
	isStopped    bool
	lastTick     time.Time
	autoCreate   bool
	topicCreated bool
}

// IsStopped checks that the consumer is not handling any messages and has stopped consuming
func (consumer *MessageHandler) IsStopped() bool {
	return !consumer.isHandling && !consumer.isConsuming
}

// AutoCreate tells the group to create the topic when it can
func (consumer *MessageHandler) AutoCreate() {
	consumer.autoCreate = true
}

// Topic returns the topic of the message handler
func (consumer *MessageHandler) Topic() string {
	return avro.KeyToTopic[consumer.bundle.Key()]
}

// Stop stops the consumer from consuming any more batches
func (consumer *MessageHandler) Stop() {
	ctx := consumer.Context()
	logger, err := appctx.GetLogger(ctx)
	if err != nil {
		_, logger = logging.SetupLogger(ctx)
	}
	logger.Debug().
		Str("topic", consumer.Topic()).
		Msg("stopping")
	consumer.isStopped = true
}

// Read reads messages from the kafka connection
func (consumer *MessageHandler) Read(conn *kafka.Conn) ([]kafka.Message, error) {
	msgs := []kafka.Message{}
	// not sure if these are the correct constraints to provide
	batch := conn.ReadBatch(1, 1e6) // we should never hit this limit
	for {
		if len(msgs) == consumer.batchLimit {
			// limit has been met
			break
		}
		msg, err := batch.ReadMessage()
		if err != nil {
			if err == io.EOF {
				// batch is finished reading
				break
			}
			errs := []error{err}
			e := batch.Close()
			if e != nil {
				errs = append(errs, e)
			}
			return msgs, &errorutils.MultiError{
				Errs: errs,
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, batch.Close()
}

// Context returns the context from the service
func (consumer *MessageHandler) Context() context.Context {
	return consumer.service.Context()
}

// Commit commits messages that have been read and inserted
func Commit(committer Committer, msgs ...kafka.Message) error {
	mapping := make(map[string]map[int]int64)
	for _, msg := range msgs {
		if mapping[msg.Topic] == nil {
			mapping[msg.Topic] = make(map[int]int64)
		}
		if msg.Offset > mapping[msg.Topic][msg.Partition] {
			mapping[msg.Topic][msg.Partition] = msg.Offset
		}
	}
	return committer.CommitOffsets(mapping)
}

func handleMessages(ctx context.Context, consumer *MessageHandler, msgs []kafka.Message) error {
	handler := Handlers[consumer.bundle.Key()]
	if handler == nil {
		return errors.New("unknown handler asked for during message handle")
	}
	return handler(ctx, consumer, msgs...)
}

// HandleAndCommit runs the handler and commits to the kafka queue in one database transaction
func (consumer *MessageHandler) HandleAndCommit(
	ctx context.Context,
	logger *zerolog.Logger,
	committer Committer,
	msgs []kafka.Message,
) error {
	ds := consumer.service.Datastore()
	// this is all one db transaction we do not want to commit to db
	// until we successfully commit to the consumer group (Committer)
	ctx, _, err := ds.ResolveConnection(ctx)
	if err != nil {
		return errorutils.Wrap(err, "during tx formation")
	}
	logger.Info().
		Str("topic", consumer.Topic()).
		Int("count", len(msgs)).
		Msg("handling messages")
	if err := handleMessages(ctx, consumer, msgs); err != nil {
		return errorutils.Wrap(err, "during handler")
	}
	if err := Commit(committer, msgs...); err != nil {
		return errorutils.Wrap(err, "during msg commit")
	}
	if err := ds.Commit(ctx); err != nil {
		return errorutils.Wrap(err, "during tx commit")
	}
	return nil
}

// IdleFor returns notes whether or not the connection is temporarily done consuming
func (consumer *MessageHandler) IdleFor(duration time.Duration) bool {
	return !consumer.isHandling && (consumer.lastTick.IsZero() || time.Now().After(consumer.lastTick.Add(duration)))
}

// Tick check if the provided context is done
func (consumer *MessageHandler) Tick(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Microsecond):
		return true
	}
}

// TopicCreated checks if the topic was created
func (consumer *MessageHandler) TopicCreated() bool {
	return consumer.topicCreated
}

// ConsumeAssignedPartitionSync consumes the assigned partition
func (consumer *MessageHandler) ConsumeAssignedPartitionSync(
	ctx context.Context,
	erred chan error,
	generation Committer,
	topic string,
	assignment kafka.PartitionAssignment,
) error {
	logger, _ := appctx.GetLogger(consumer.Context())
	logger.Debug().
		Str("topic", topic).
		Int("partition-id", assignment.ID).
		Int64("offset", assignment.Offset).
		Msg("received assignments")
	broker := kafkautils.Brokers(consumer.Context())[0]
	connection, err := NewConnection(ctx, consumer.service.dialer, broker, topic, assignment)
	if err != nil {
		return err
	}
	if consumer.autoCreate {
		if err := connection.AutoCreate(); err != nil {
			return err
		}
	}

	consumer.topicCreated = true
	if err := connection.Seek(); err != nil {
		return err
	}
	defer connection.Close(erred)
	consumer.isStopped = true
	for { // loop to continue consuming
		if !consumer.isStopped {
			logger.Info().Msg("breaking consumption, consumer no longer marked as consuming")
			break
		}
		// check to make sure nothiing else has closed the context
		if ok := consumer.Tick(ctx); !ok {
			logger.Info().Msg("breaking consumption, context marked as done")
			break
		}
		consumer.isHandling = false
		msgs, e := consumer.Read(connection.conn)
		if e != nil {
			return errorutils.Wrap(e, "during read")
		}
		if len(msgs) == 0 {
			continue
		}

		consumer.isHandling = true
		consumer.lastTick = time.Now()
		if e := consumer.HandleAndCommit(ctx, logger, generation, msgs); e != nil {
			return errorutils.Wrap(e, "within handle and commit")
		}
		consumer.lastTick = time.Now()
	}
	return nil
}

// ConsumeAssignedPartition consumes the assigned partition
func (consumer *MessageHandler) ConsumeAssignedPartition(
	ctx context.Context,
	erred chan error,
	generation Committer,
	topic string,
	assignment kafka.PartitionAssignment,
) {
	err := consumer.ConsumeAssignedPartitionSync(
		ctx,
		erred,
		generation,
		topic,
		assignment,
	)
	consumer.isHandling = false
	consumer.isConsuming = false
	if err != nil {
		erred <- errorutils.Wrap(
			err,
			fmt.Sprintf("error in topic - %s", consumer.Topic()),
		)
	} else {
		erred <- nil
	}
}

// CloseGroup closes a service group
func (service *Service) CloseGroup() error {
	if service.group == nil {
		return nil
	}
	group := service.group
	service.group = nil
	return group.Close()
}

// Topics returns a list of topics
func (service *Service) Topics() []string {
	topics := []string{}
	for key := range service.consumers {
		topics = append(topics, avro.KeyToTopic[key])
	}
	return topics
}

// NextGroup creates the next generation
func (service *Service) NextGroup() (*kafka.Generation, error) {
	if service.group == nil {
		group, _, err := kafkautils.InitKafkaConsumerGroup(
			service.Context(),
			service.Topics(),
			service.dialer,
		)
		if err != nil {
			return nil, err
		}
		service.group = group
	}
	return service.group.Next(service.Context())
}

func resolveError(service *Service, err error) error {
	if errors.Is(err, kafka.RebalanceInProgress) {
		if e := service.CloseGroup(); e != nil {
			return errorutils.Wrap(&errorutils.MultiError{
				Errs: []error{err, e},
			}, "unable to close group")
		}
		return errorutils.Wrap(err, "consumer group is being rebalanced, closed group")
	}
	if errors.Is(err, kafka.ErrGroupClosed) {
		service.group = nil
	}
	return errorutils.Wrap(err, "consumer group cannot be joined right now")
}

// JoinGroup starts the consumers
func (service *Service) JoinGroup(
	generation *kafka.Generation,
) chan error {
	erred := make(chan error)
	generation.Start(func(ctx context.Context) {
		innerErr := make(chan error)
		for topic, partitionAssignments := range generation.Assignments {
			consumer := service.ConsumerByTopic(topic)
			if consumer == nil {
				continue
			}
			consumer.isConsuming = true
			for _, partitionAssignment := range partitionAssignments {
				// should only be one, but may be more than if partitions mis-match
				go consumer.ConsumeAssignedPartition(ctx, innerErr, generation, topic, partitionAssignment)
			}
		}
		select {
		case err := <-innerErr:
			erred <- err
		case <-ctx.Done():
			service.StopConsumers()
		}
	})
	return erred
}

// ConsumerByTopic finds the consumer that matches a given topic
func (service *Service) ConsumerByTopic(topic string) *MessageHandler {
	var consumer *MessageHandler
	for key := range service.consumers {
		if service.consumers[key].Topic() == topic {
			consumer = service.consumers[key].(*MessageHandler)
			break
		}
	}
	return consumer
}

// Produce produces messages
func (consumer *MessageHandler) Produce(
	ctx context.Context,
	encoder avro.TopicBundle,
	encodables ...interface{},
) error {
	if len(encodables) == 0 {
		return nil
	}
	messages, err := encoder.ManyToBinary(encodables...)
	if err != nil {
		return err
	}
	return consumer.writer.WriteMessages(
		ctx,
		*messages...,
	)
}

// WithTopicAutoCreation creates topics used by producers and consumers
func WithTopicAutoCreation(service *Service) error {
	if service.producers != nil {
		for key := range service.producers {
			service.producers[key].AutoCreate()
		}
	}
	setupConsumerGroup := false
	if service.consumers != nil {
		for key := range service.consumers {
			service.consumers[key].AutoCreate()
			setupConsumerGroup = true
		}
	}
	if !setupConsumerGroup {
		return nil
	}
	errChan := service.Consume()
	for {
		select {
		case err := <-errChan:
			return err
		case <-time.After(time.Second):
		}
		if service.AllTopicsCreated() {
			break
		}
	}
	return nil
}

// AllTopicsCreated checks whether all of the topics have been checked as created
func (service *Service) AllTopicsCreated() bool {
	for _, consumer := range service.consumers {
		if !consumer.TopicCreated() {
			return false
		}
	}
	return true
}

// Consumer retrieves a consumer off of the service
func (service *Service) Consumer(key string) BatchMessageConsumer {
	return service.consumers[key]
}

// Producer returns a kafka message producer
func (service *Service) Producer(key string) BatchMessageProducer {
	return service.producers[key]
}

// WithConsumer sets up a consumer on the service
func WithConsumer(
	batchLimit int,
	topicKeys ...string,
) func(*Service) error {
	return func(service *Service) error {
		if service.consumers == nil {
			// can't access consumer keys until make is called
			service.consumers = make(map[string]BatchMessageConsumer)
		}
		for _, topicKey := range topicKeys {
			consumer := &MessageHandler{
				key:        topicKey,
				bundle:     KeyToEncoder[topicKey],
				service:    service,
				batchLimit: batchLimit,
			}
			service.consumers[topicKey] = BatchMessageConsumer(consumer)
		}
		group, dialer, err := kafkautils.InitKafkaConsumerGroup(
			service.Context(),
			service.Topics(),
			service.dialer,
		)
		if err != nil {
			return err
		}
		service.group = group
		service.dialer = dialer
		return nil
	}
}

// WithProducer sets up a consumer on the service
func WithProducer(
	topicKeys ...string,
) func(*Service) error {
	return func(service *Service) error {
		if service.producers == nil {
			// can't access producer keys until make is called
			service.producers = make(map[string]BatchMessageProducer)
		}
		dialer := service.dialer
		for _, topicKey := range topicKeys {
			writer, d, err := kafkautils.InitKafkaWriter(
				service.Context(),
				avro.KeyToTopic[topicKey],
				dialer,
			)
			if err != nil {
				return err
			}
			dialer = d
			producer := &MessageHandler{
				key:     topicKey,
				bundle:  KeyToEncoder[topicKey],
				writer:  writer,
				dialer:  dialer,
				service: service,
			}
			service.dialer = dialer
			service.producers[topicKey] = BatchMessageProducer(producer)
		}
		return nil
	}
}
