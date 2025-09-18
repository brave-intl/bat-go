package xslack

import "context"

type MockClient struct {
	FnSendMessage func(ctx context.Context, msg *Message) error
}

func (c *MockClient) SendMessage(ctx context.Context, msg *Message) error {
	if c.FnSendMessage == nil {
		return nil
	}

	return c.FnSendMessage(ctx, msg)
}
