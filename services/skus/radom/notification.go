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

type Notification struct {
	EventType string     `json:"eventType"`
	EventData *EventData `json:"eventData"`
	RadomData *Data      `json:"radomData"`
}

type EventData struct {
	New                   *NewSubscription                   `json:"newSubscription"`
	Payment               *SubscriptionPayment               `json:"subscriptionPayment"`
	Cancelled             *SubscriptionCancelled             `json:"subscriptionCancelled"`
	Expired               *SubscriptionExpired               `json:"subscriptionExpired"`
	PaymentAttemptFailure *SubscriptionPaymentAttemptFailure `json:"subscriptionPaymentAttemptFailure"`
	PaymentOverdue        *SubscriptionPaymentOverdue        `json:"subscriptionPaymentOverdue"`
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

type SubscriptionPaymentAttemptFailure struct {
	SubscriptionID uuid.UUID `json:"subscriptionId"`
}

type SubscriptionPaymentOverdue struct {
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

func (n *Notification) OrderID() (uuid.UUID, error) {
	switch {
	case n.EventData == nil || n.EventData.New == nil:
		return uuid.Nil, ErrUnsupportedEvent

	case n.RadomData == nil || n.RadomData.CheckoutSession == nil:
		return uuid.Nil, ErrNoCheckoutSessionData

	default:
		mdata := n.RadomData.CheckoutSession.Metadata

		for i := range mdata {
			if mdata[i].Key == "brave_order_id" {
				return uuid.FromString(mdata[i].Value)
			}
		}

		return uuid.Nil, ErrBraveOrderIDNotFound
	}
}

func (n *Notification) SubID() (uuid.UUID, error) {
	switch {
	case n.EventData == nil:
		return uuid.Nil, ErrUnsupportedEvent

	case n.EventData.New != nil:
		return n.EventData.New.SubscriptionID, nil

	case n.EventData.Payment != nil:
		if n.EventData.Payment.RadomData == nil || n.EventData.Payment.RadomData.Subscription == nil {
			return uuid.Nil, ErrNoRadomPaymentData
		}

		return n.EventData.Payment.RadomData.Subscription.SubscriptionID, nil

	case n.EventData.Cancelled != nil:
		return n.EventData.Cancelled.SubscriptionID, nil

	case n.EventData.Expired != nil:
		return n.EventData.Expired.SubscriptionID, nil

	case n.EventData.PaymentAttemptFailure != nil:
		return n.EventData.PaymentAttemptFailure.SubscriptionID, nil

	case n.EventData.PaymentOverdue != nil:
		return n.EventData.PaymentOverdue.SubscriptionID, nil

	default:
		return uuid.Nil, ErrUnsupportedEvent
	}
}

func (n *Notification) IsNewSub() bool {
	return n.EventData != nil && n.EventData.New != nil
}

func (n *Notification) ShouldRenew() bool {
	return n.EventData != nil && n.EventData.Payment != nil
}

func (n *Notification) ShouldCancel() bool {
	switch {
	case n.EventData == nil:
		return false

	case n.EventData.Cancelled != nil:
		return true

	case n.EventData.Expired != nil:
		return true

	default:
		return false
	}
}

func (n *Notification) ShouldRecordPayFailure() bool {
	switch {
	case n.EventData == nil:
		return false

	case n.EventData.PaymentAttemptFailure != nil:
		return true

	case n.EventData.PaymentOverdue != nil:
		return true

	default:
		return false
	}
}

func (n *Notification) ShouldProcess() bool {
	return n.IsNewSub() || n.ShouldRenew() || n.ShouldCancel() || n.ShouldRecordPayFailure()
}

func (n *Notification) Effect() string {
	switch {
	case n.IsNewSub():
		return "new"

	case n.ShouldRenew():
		return "renew"

	case n.ShouldCancel():
		return "cancel"

	case n.ShouldRecordPayFailure():
		return "payment_failure"

	default:
		return "skip"
	}
}

func (n *Notification) NtfType() string {
	return n.EventType
}

func ParseNotification(b []byte) (*Notification, error) {
	ntf := &Notification{}
	if err := json.Unmarshal(b, ntf); err != nil {
		return nil, err
	}

	return ntf, nil
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
