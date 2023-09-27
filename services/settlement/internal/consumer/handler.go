package consumer

import (
	"context"
	"fmt"

	"github.com/brave-intl/bat-go/services/settlement/internal/consumer/redis"
)

// XAdder TODO.
type XAdder interface {
	XAdd(ctx context.Context, args redis.XAddArgs) error
}

// MessageFactory defines a function to create new Message's.
type MessageFactory func(body interface{}) (Message, error)

type DLQHandler struct {
	adder  XAdder
	conf   Config
	dlq    string
	newMsg MessageFactory
}

func NewDLQHandler(xAdder XAdder, conf Config, dlq string, newMsg MessageFactory) *DLQHandler {
	return &DLQHandler{
		adder:  xAdder,
		conf:   conf,
		dlq:    dlq,
		newMsg: newMsg,
	}
}

func (d *DLQHandler) Handle(ctx context.Context, xMsg redis.XMessage, processingError error) error {
	const (
		c   = "x-err-on-consumer-id"
		cg  = "x-err-on-consumer-group"
		sn  = "x-err-on-stream"
		msg = "x-err-message"
	)

	m, err := d.newMsg(xMsg.Values)
	if err != nil {
		return fmt.Errorf("error creating new dlq message: %w", err)
	}

	m.SetHeader(c, d.conf.consumerID)
	m.SetHeader(cg, d.conf.consumerGroup)
	m.SetHeader(sn, d.conf.streamName)
	m.SetHeader(msg, processingError.Error())

	err = d.adder.XAdd(ctx, redis.XAddArgs{
		Stream: d.dlq,
		Values: map[string]interface{}{dataKey: m},
	})
	if err != nil {
		return fmt.Errorf("error sending dlq message: %w", err)
	}

	return nil
}
