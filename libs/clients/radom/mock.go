package radom

import (
	"context"
)

type MockClient struct {
	fnCreateCheckoutSession func(ctx context.Context, req *CheckoutSessionRequest) (*CheckoutSessionResponse, error)
}

func (c *MockClient) CreateCheckoutSession(
	ctx context.Context,
	req *CheckoutSessionRequest,
) (*CheckoutSessionResponse, error) {
	if c.fnCreateCheckoutSession == nil {
		return &CheckoutSessionResponse{}, nil
	}

	return c.fnCreateCheckoutSession(ctx, req)
}
