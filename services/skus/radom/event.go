package radom

import (
	"context"
	"crypto/subtle"
	"encoding/json"

	uuid "github.com/satori/go.uuid"
)

const (
	ErrUnsupportedEvent       = Error("radom: unsupported event type for brave order id")
	ErrBraveOrderIDNotFound   = Error("radom: brave order id not found")
	ErrSubscriptionIDNotFound = Error("radom: subscription id not found")

	ErrDisabled               = Error("radom: radom disabled")
	ErrVerificationKeyEmpty   = Error("radom: verification key is empty")
	ErrVerificationKeyInvalid = Error("radom: verification key is invalid")
)

type Event struct {
	EventType string    `json:"eventType"`
	EventData EventData `json:"eventData"`
	RadomData RadData   `json:"radomData"`
}

func (e *Event) OrderID() (uuid.UUID, error) {
	if e.EventData.NewSubscription == nil {
		return uuid.Nil, ErrUnsupportedEvent
	}

	mdata := e.RadomData.CheckoutSession.Metadata

	for i := range mdata {
		d := mdata[i]
		if d.Key == "brave_order_id" {
			return uuid.FromString(d.Value)
		}
	}

	return uuid.Nil, ErrBraveOrderIDNotFound
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
		return uuid.Nil, ErrSubscriptionIDNotFound
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

func ParseEvent(b []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(b, &event); err != nil {
		return Event{}, err
	}

	return event, nil
}

type MessageAuthConfig struct {
	Token   []byte
	Enabled bool
}

type MessageAuthenticator struct {
	cfg MessageAuthConfig
}

func NewMessageAuthenticator(cfg MessageAuthConfig) *MessageAuthenticator {
	return &MessageAuthenticator{
		cfg: cfg,
	}
}

func (r *MessageAuthenticator) Authenticate(_ context.Context, token string) error {
	if !r.cfg.Enabled {
		return ErrDisabled
	}

	if token == "" {
		return ErrVerificationKeyEmpty
	}

	if subtle.ConstantTimeCompare(r.cfg.Token, []byte(token)) != 1 {
		return ErrVerificationKeyInvalid
	}

	return nil
}
