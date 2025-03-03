package xstripe

import (
	"context"

	"github.com/stripe/stripe-go/v72"
)

type MockClient struct {
	FnSession       func(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	FnCreateSession func(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error)
	FnSubscription  func(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error)
	FnFindCustomer  func(ctx context.Context, email string) (*stripe.Customer, bool)
	FnCancelSub     func(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error
}

func (c *MockClient) Session(ctx context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	if c.FnSession == nil {
		result := &stripe.CheckoutSession{
			ID:       id,
			Customer: &stripe.Customer{ID: "cus_id", Email: "customer@example.com"},
		}

		return result, nil
	}

	return c.FnSession(ctx, id, params)
}

func (c *MockClient) CreateSession(ctx context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	if c.FnCreateSession == nil {
		result := &stripe.CheckoutSession{
			ID:                 "cs_test_id",
			PaymentMethodTypes: []string{"card"},
			Mode:               stripe.CheckoutSessionModeSubscription,
			SuccessURL:         *params.SuccessURL,
			CancelURL:          *params.CancelURL,
			ClientReferenceID:  *params.ClientReferenceID,
			Subscription: &stripe.Subscription{
				ID: "sub_id",
				Metadata: map[string]string{
					"orderID": *params.ClientReferenceID,
				},
			},
			AllowPromotionCodes: true,
		}

		return result, nil
	}

	return c.FnCreateSession(ctx, params)
}

func (c *MockClient) Subscription(ctx context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	if c.FnSubscription == nil {
		result := &stripe.Subscription{
			ID: id,
		}

		return result, nil
	}

	return c.FnSubscription(ctx, id, params)
}

func (c *MockClient) FindCustomer(ctx context.Context, email string) (*stripe.Customer, bool) {
	if c.FnFindCustomer == nil {
		result := &stripe.Customer{
			ID:    "cus_id",
			Email: email,
		}

		return result, true
	}

	return c.FnFindCustomer(ctx, email)
}

func (c *MockClient) CancelSub(ctx context.Context, id string, params *stripe.SubscriptionCancelParams) error {
	if c.FnCancelSub == nil {
		return nil
	}

	return c.FnCancelSub(ctx, id, params)
}
