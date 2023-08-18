package radom

import (
	"context"
)

type MockClient struct {
	FnCreateCheckoutSession func(ctx context.Context, req *CheckoutSessionRequest) (*CheckoutSessionResponse, error)
}

func (c *MockClient) CreateCheckoutSession(
	ctx context.Context,
	req *CheckoutSessionRequest,
) (*CheckoutSessionResponse, error) {
	if c.FnCreateCheckoutSession == nil {
		return &CheckoutSessionResponse{}, nil
	}

	return c.FnCreateCheckoutSession(ctx, req)
}
