package radom

import (
	"context"
	"crypto/subtle"
	"encoding/json"

	uuid "github.com/satori/go.uuid"
)

const (
	ErrUnsupportedEvent      = Error("radom: unsupported event")
	ErrNoCheckoutSessionData = Error("radom: no checkout session data")
	ErrBraveOrderIDNotFound  = Error("radom: brave order id not found")
	ErrNoRadomPaymentData    = Error("radom: no radom payment data")

	ErrDisabled               = Error("radom: disabled")
	ErrVerificationKeyEmpty   = Error("radom: verification key is empty")
	ErrVerificationKeyInvalid = Error("radom: verification key is invalid")
)

type Event struct {
	EventType string     `json:"eventType"`
	EventData *EventData `json:"eventData"`
	RadomData *Data      `json:"radomData"`
}

type EventData struct {
	New       *NewSubscription       `json:"newSubscription"`
	Payment   *SubscriptionPayment   `json:"subscriptionPayment"`
	Cancelled *SubscriptionCancelled `json:"subscriptionCancelled"`
	Expired   *SubscriptionExpired   `json:"subscriptionExpired"`
}

type NewSubscription struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type SubscriptionPayment struct {
	RadomData *Data `json:"radomData"`
}

type SubscriptionCancelled struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type SubscriptionExpired struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type Data struct {
	CheckoutSession *CheckoutSession `json:"checkoutSession"`
	Subscription    *Subscription    `json:"subscription"`
}

type CheckoutSession struct {
	CheckoutSessionID string     `json:"checkoutSessionId"`
	Metadata          []Metadata `json:"metadata"`
}

type Subscription struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

func (e *Event) OrderID() (uuid.UUID, error) {
	switch {
	case e.EventData == nil || e.EventData.New == nil:
		return uuid.Nil, ErrUnsupportedEvent

	case e.RadomData == nil || e.RadomData.CheckoutSession == nil:
		return uuid.Nil, ErrNoCheckoutSessionData

	default:
		mdata := e.RadomData.CheckoutSession.Metadata

		for i := range mdata {
			d := mdata[i]
			if d.Key == "brave_order_id" {
				return uuid.FromString(d.Value)
			}
		}

		return uuid.Nil, ErrBraveOrderIDNotFound
	}
}

func (e *Event) SubID() (uuid.UUID, error) {
	switch {
	case e.EventData == nil:
		return uuid.Nil, ErrUnsupportedEvent

	case e.EventData.New != nil:
		return e.EventData.New.SubscriptionID, nil

	case e.EventData.Payment != nil:
		if e.EventData.Payment.RadomData == nil || e.EventData.Payment.RadomData.Subscription == nil {
			return uuid.Nil, ErrNoRadomPaymentData
		}

		return e.EventData.Payment.RadomData.Subscription.SubscriptionID, nil

	case e.EventData.Cancelled != nil:
		return e.EventData.Cancelled.SubscriptionID, nil

	case e.EventData.Expired != nil:
		return e.EventData.Expired.SubscriptionID, nil

	default:
		return uuid.Nil, ErrUnsupportedEvent
	}
}

func (e *Event) IsNewSub() bool {
	return e.EventData != nil && e.EventData.New != nil
}

func (e *Event) ShouldRenew() bool {
	return e.EventData != nil && e.EventData.Payment != nil
}

func (e *Event) ShouldCancel() bool {
	switch {
	case e.EventData == nil:
		return false

	case e.EventData.Cancelled != nil:
		return true

	case e.EventData.Expired != nil:
		return true

	default:
		return false
	}
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

func ParseEvent(b []byte) (*Event, error) {
	event := &Event{}
	if err := json.Unmarshal(b, event); err != nil {
		return nil, err
	}

	return event, nil
}

type MessageAuthConfig struct {
	Enabled bool
	Token   []byte
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
