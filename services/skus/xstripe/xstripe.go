package xstripe

import (
	"context"

	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/client"
	"github.com/stripe/stripe-go/v72/customer"
)

type Client struct {
	cl *client.API
}

func NewClient(cl *client.API) *Client {
	return &Client{cl: cl}
}

func (c *Client) Session(_ context.Context, id string, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	return c.cl.CheckoutSessions.Get(id, params)
}

func (c *Client) CreateSession(_ context.Context, params *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
	return c.cl.CheckoutSessions.New(params)
}

func (c *Client) Subscription(_ context.Context, id string, params *stripe.SubscriptionParams) (*stripe.Subscription, error) {
	return c.cl.Subscriptions.Get(id, params)
}

func (c *Client) FindCustomer(ctx context.Context, email string) (*stripe.Customer, bool) {
	iter := c.Customers(ctx, &stripe.CustomerListParams{Email: &email})

	for iter.Next() {
		return iter.Customer(), true
	}

	return nil, false
}

func (c *Client) Customers(_ context.Context, params *stripe.CustomerListParams) *customer.Iter {
	return c.cl.Customers.List(params)
}

func (c *Client) CancelSub(_ context.Context, id string, params *stripe.SubscriptionCancelParams) error {
	_, err := c.cl.Subscriptions.Cancel(id, params)

	return err
}

func CustomerEmailFromSession(sess *stripe.CheckoutSession) string {
	// Use the customer email if the customer has completed the payment flow.
	if sess.Customer != nil && sess.Customer.Email != "" {
		return sess.Customer.Email
	}

	// This is unlikely to be set, but in case it is, use it.
	if sess.CustomerEmail != "" {
		return sess.CustomerEmail
	}

	// Default to empty, Stripe will ask the customer.
	return ""
}

func CustomerIDFromSession(sess *stripe.CheckoutSession) string {
	// Return the customer id only if the customer is present AND it has email set.
	// Without the email, the customer record is not fully formed, and does not suit the use case.
	if sess.Customer != nil && sess.Customer.Email != "" {
		return sess.Customer.ID
	}

	return ""
}
