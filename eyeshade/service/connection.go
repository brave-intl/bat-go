package eyeshade

import (
	"context"
	"fmt"
	"os"
	"strconv"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/segmentio/kafka-go"
)

// Connection creates a connection abstraction object to help maintain metadata about a connection
type Connection struct {
	dialer     *kafka.Dialer
	conn       *kafka.Conn
	topic      string
	assignment kafka.PartitionAssignment
}

// Close closes a connection and sends back the error through a cahnnel
func (connection *Connection) Close(erred chan error) {
	err := connection.conn.Close()
	if err != nil {
		erred <- err
	}
}

// DialBrokerController dials the brokers to find the controller address
func (connection *Connection) DialBrokerController() (*kafka.Conn, error) {
	brokers, err := connection.conn.Brokers()
	if err != nil {
		return nil, errorutils.Wrap(err, "unable to obtain brokers")
	}
	errs := []error{}
	for _, broker := range brokers {
		conn, err := connection.dialer.Dial("tcp", broker.Host+":"+fmt.Sprintf("%d", broker.Port))
		if err != nil {
			errs = append(errs, err)
			if err := conn.Close(); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		controlBroker, err := conn.Controller()
		if err != nil {
			errs = append(errs, err)
			if err := conn.Close(); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		controllerURL := controlBroker.Host + ":" + fmt.Sprintf("%d", controlBroker.Port)
		controllerConn, err := connection.dialer.Dial("tcp", controllerURL)
		if err != nil {
			errs = append(errs, err)
			if err := conn.Close(); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		return controllerConn, conn.Close()
	}
	return nil, &errorutils.MultiError{
		Errs: errs,
	}
}

// AutoCreate runs through the auto create process for a topic
func (connection *Connection) AutoCreate() error {
	topic := connection.topic
	conn, err := connection.DialBrokerController()
	if err != nil {
		return errorutils.Wrap(err, "unable to provide broker controller")
	}
	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return err
	}
	if len(partitions) != 0 {
		return nil // topic created
	}
	reps, err := strconv.Atoi(os.Getenv("KAFKA_REPLICATIONS"))
	if err != nil {
		return err
	}
	requestedPartitions, err := strconv.Atoi(os.Getenv("KAFKA_PARTITIONS"))
	if err != nil {
		return err
	}

	return conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     requestedPartitions,
		ReplicationFactor: reps,
	})
}

// Seek moves the topic to the assignment's offset
func (connection *Connection) Seek() error {
	offset := connection.assignment.Offset
	if offset > -2 {
		_, err := connection.conn.Seek(offset+1, kafka.SeekDontCheck)
		if err != nil {
			return errorutils.Wrap(err, "failed to seek")
		}
	}
	return nil
}

// NewConnection creates a new connection object
func NewConnection(
	ctx context.Context,
	dialer *kafka.Dialer,
	address string,
	topic string,
	assignment kafka.PartitionAssignment,
) (*Connection, error) {
	conn, err := dialer.DialLeader(ctx, "tcp", address, topic, assignment.ID)
	if err != nil {
		return nil, errorutils.Wrap(err, "failed to dial leader")
	}
	return &Connection{
		dialer:     dialer,
		conn:       conn,
		topic:      topic,
		assignment: assignment,
	}, nil
}
