package skus

import (
	"encoding/json"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go/v72"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errStripeSkipEvent        = model.Error("stripe: skip webhook event")
	errStripeUnsupportedEvent = model.Error("stripe: unsupported webhook event")
	errStripeNoInvoiceSub     = model.Error("strupe: no invoice subscription")
	errStripeNoInvoiceLines   = model.Error("stripe: no invoice lines")
	errStripeOrderIDMissing   = model.Error("stripe: order_id missing")
	errStripeInvalidSubPeriod = model.Error("stripe: invalid subscription period")
)

type stripeNotification struct {
	raw     *stripe.Event
	invoice *stripe.Invoice
	sub     *stripe.Subscription
}

func parseStripeNotification(raw *stripe.Event) (*stripeNotification, error) {
	result := &stripeNotification{
		raw: raw,
	}

	switch raw.Type {
	case "invoice.paid":
		val, err := parseStripeEventData[stripe.Invoice](raw.Data.Raw)
		if err != nil {
			return nil, err
		}

		result.invoice = val

		return result, nil

	case "customer.subscription.deleted":
		val, err := parseStripeEventData[stripe.Subscription](raw.Data.Raw)
		if err != nil {
			return nil, err
		}

		result.sub = val

		return result, nil

	default:
		return nil, errStripeSkipEvent
	}
}

func (x *stripeNotification) shouldProcess() bool {
	return x.shouldRenew() || x.shouldCancel()
}

func (x *stripeNotification) shouldRenew() bool {
	return x.invoice != nil && x.raw.Type == "invoice.paid"
}

func (x *stripeNotification) shouldCancel() bool {
	return x.sub != nil && x.raw.Type == "customer.subscription.deleted"
}

func (x *stripeNotification) ntfType() string {
	return x.raw.Type
}

func (x *stripeNotification) ntfSubType() string {
	switch {
	case x.invoice != nil && x.sub == nil:
		return "invoice"

	case x.sub != nil && x.invoice == nil:
		return "subscription"

	default:
		return "unknown"
	}
}

func (x *stripeNotification) effect() string {
	switch {
	case x.shouldRenew():
		return "renew"

	case x.shouldCancel():
		return "cancel"

	default:
		return "skip"
	}
}

func (x *stripeNotification) subID() (string, error) {
	switch {
	case x.invoice != nil:
		if x.invoice.Subscription == nil {
			return "", errStripeNoInvoiceSub
		}

		return x.invoice.Subscription.ID, nil

	case x.sub != nil:
		return x.sub.ID, nil

	default:
		return "", errStripeUnsupportedEvent
	}
}

func (x *stripeNotification) orderID() (uuid.UUID, error) {
	switch {
	case x.invoice != nil:
		if x.invoice.Lines == nil || len(x.invoice.Lines.Data) == 0 {
			return uuid.Nil, errStripeNoInvoiceLines
		}

		id, ok := x.invoice.Lines.Data[0].Metadata["orderID"]
		if !ok {
			return uuid.Nil, errStripeOrderIDMissing
		}

		return uuid.FromString(id)

	case x.sub != nil:
		id, ok := x.sub.Metadata["orderID"]
		if !ok {
			return uuid.Nil, errStripeOrderIDMissing
		}

		return uuid.FromString(id)

	default:
		return uuid.Nil, errStripeUnsupportedEvent
	}
}

func (x *stripeNotification) expiresTime() (time.Time, error) {
	if x.invoice == nil {
		return time.Time{}, errStripeUnsupportedEvent
	}

	if x.invoice.Lines == nil || len(x.invoice.Lines.Data) == 0 {
		return time.Time{}, errStripeNoInvoiceLines
	}

	sub := x.invoice.Lines.Data[0]
	if sub.Period == nil || sub.Period.End == 0 {
		return time.Time{}, errStripeInvalidSubPeriod
	}

	return time.Unix(sub.Period.End, 0).UTC(), nil
}

func parseStripeEventData[T any](data []byte) (*T, error) {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
