package radom

import (
	"context"

	"github.com/brave-intl/bat-go/services/skus/model"
	uuid "github.com/satori/go.uuid"
)

type Event struct {
	EventType string    `json:"eventType"`
	EventData EventData `json:"eventData"`
	RadomData RadData   `json:"radomData"`
}

func (e *Event) OrderID() (uuid.UUID, error) {
	if e.EventData.NewSubscription == nil {
		return uuid.Nil, model.Error("radom: unsupported event type for brave order id")
	}

	mdata := e.EventData.NewSubscription.RadomData.CheckoutSession.Metadata

	for i := range mdata {
		d := mdata[i]
		if d.Key == "brave_order_id" {
			return uuid.FromString(d.Value)
		}
	}

	return uuid.Nil, model.Error("radom: brave order id not found")
}

func (e *Event) SubID() (uuid.UUID, error) {
	var subID uuid.UUID

	switch {
	case e.EventData.NewSubscription != nil:
		subID = e.EventData.NewSubscription.SubscriptionID

	case e.EventData.SubscriptionPayment != nil:
		subID = e.EventData.SubscriptionPayment.RadomData.Subscription.SubscriptionID

	case e.EventData.SubscriptionCancelled != nil:
		subID = e.EventData.SubscriptionCancelled.SubscriptionID

	case e.EventData.SubscriptionExpired != nil:
		subID = e.EventData.SubscriptionExpired.SubscriptionID
	}

	if uuid.Equal(subID, uuid.Nil) {
		return uuid.Nil, model.Error("radom: radom subscription id not found")
	}

	return subID, nil
}

func (e *Event) IsNewSub() bool {
	return e.EventData.NewSubscription != nil
}

func (e *Event) ShouldRenew() bool {
	return e.EventData.SubscriptionPayment != nil
}

func (e *Event) ShouldCancel() bool {
	return e.EventData.SubscriptionCancelled != nil || e.EventData.SubscriptionExpired != nil
}

func (e *Event) ShouldProcess() bool {
	return e.IsNewSub() || e.ShouldRenew() || e.ShouldCancel()
}

func (e *Event) Effect() string {
	switch {
	case e.IsNewSub():
		return "new"

	case e.ShouldRenew():
		return "renew"

	case e.ShouldCancel():
		return "cancel"

	default:
		return "skip"
	}
}

type EventData struct {
	NewSubscription       *NewSubscription       `json:"newSubscription"`
	SubscriptionPayment   *SubscriptionPayment   `json:"subscriptionPayment"`
	SubscriptionCancelled *SubscriptionCancelled `json:"subscriptionCancelled"`
	SubscriptionExpired   *SubscriptionExpired   `json:"subscriptionExpired"`
}

type NewSubscription struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
	RadomData      RadData   `json:"radomData"`
}

type SubscriptionPayment struct {
	RadomData RadData `json:"radomData"`
}

type SubscriptionCancelled struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type SubscriptionExpired struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type RadData struct {
	CheckoutSession CheckoutSession `json:"checkoutSession"`
	Subscription    Subscription    `json:"subscription"`
}

type CheckoutSession struct {
	CheckoutSessionID string     `json:"checkoutSessionId"`
	Metadata          []Metadata `json:"metadata"`
}

type Subscription struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type MessageAuthConfig struct {
	token    string
	disabled bool
}

type MessageAuthenticator struct {
	cfg MessageAuthConfig
}

func NewMessageAuthenticator(cfg MessageAuthConfig) *MessageAuthenticator {
	return &MessageAuthenticator{
		cfg: cfg,
	}
}

const (
	ErrDisabled             = model.Error("radom: radom disabled")
	ErrVerificationKeyEmpty = model.Error("radom: verification key is empty")
	ErrWebhookInvalidKey    = model.Error("radom: verification key is invalid")
)

func (r *MessageAuthenticator) Authenticate(_ context.Context, token string) error {
	if r.cfg.disabled {
		return ErrDisabled
	}

	if token == "" {
		return ErrVerificationKeyEmpty
	}

	if token != r.cfg.token {
		return ErrWebhookInvalidKey
	}

	return nil
}
