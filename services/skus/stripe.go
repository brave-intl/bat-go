package skus

import (
	"encoding/json"
	"time"

	uuid "github.com/satori/go.uuid"
	"github.com/stripe/stripe-go/v72"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	errStripeSkipEvent         = model.Error("stripe: skip webhook event")
	errStripeUnsupportedEvent  = model.Error("stripe: unsupported webhook event")
	errStripeNoInvoiceSub      = model.Error("stripe: no invoice subscription")
	errStripeNoInvoiceLines    = model.Error("stripe: no invoice lines")
	errStripeOrderIDMissing    = model.Error("stripe: order_id missing")
	errStripeInvalidSubPeriod  = model.Error("stripe: invalid subscription period")
	errStripeIncompleteUMAData = model.Error("stripe: incomplete upgrade monthly to annual data")
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
	case "invoice.paid", "invoice.payment_failed":
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
	return x.shouldRenew() || x.shouldCancel() || x.shouldRecordPayFailure()
}

func (x *stripeNotification) shouldRenew() bool {
	return x.invoice != nil && x.raw.Type == "invoice.paid"
}

func (x *stripeNotification) shouldCancel() bool {
	return x.sub != nil && x.raw.Type == "customer.subscription.deleted"
}

func (x *stripeNotification) shouldRecordPayFailure() bool {
	return x.invoice != nil && x.raw.Type == "invoice.payment_failed"
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

	case x.shouldRecordPayFailure():
		return "record_payment_failure"

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

func (x *stripeNotification) hasCoupon() bool {
	return x.invoice != nil && x.invoice.Discount != nil && x.invoice.Discount.Coupon != nil
}

func (x *stripeNotification) umaData() (promoMonthlyAnnualData, error) {
	if x.invoice.Lines == nil || len(x.invoice.Lines.Data) == 0 {
		return promoMonthlyAnnualData{}, errStripeNoInvoiceLines
	}

	stSubID, ok1 := x.invoice.Lines.Data[0].Metadata["uma__st_sub_id"]
	subID, ok2 := x.invoice.Lines.Data[0].Metadata["uma__sub_id"]
	ordID, ok3 := x.invoice.Lines.Data[0].Metadata["uma__order_id"]

	// For MtoA, there should be all three pieces.
	// Other combinations are invalid.
	if ok1 && ok2 && ok3 {
		result := promoMonthlyAnnualData{
			stSubID: stSubID,
			subID:   subID,
			orderID: ordID,
		}

		// Coupon is optional.
		if coupID, ok := x.invoice.Lines.Data[0].Metadata["uma__coupon_id"]; ok {
			result.coupID = coupID
		}

		return result, nil
	}

	return promoMonthlyAnnualData{}, errStripeIncompleteUMAData
}

func parseStripeEventData[T any](data []byte) (*T, error) {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

type promoMonthlyAnnualData struct {
	stSubID string
	subID   string
	orderID string
	coupID  string
}

func hasCxUsedUMACoupon(ntf *stripeNotification, promo promoMonthlyAnnualData) bool {
	if !ntf.hasCoupon() {
		return false
	}

	if promo.coupID == "" {
		return false
	}

	if ntf.invoice.Discount.Coupon.ID != promo.coupID {
		return false
	}

	return true
}
