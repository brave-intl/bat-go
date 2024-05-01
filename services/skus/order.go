package skus

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/logging"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v72"
	"gopkg.in/macaroon.v2"

	"github.com/brave-intl/bat-go/services/skus/model"
)

const (
	whStripeInvoiceUpdated          = "invoice.updated"
	whStripeInvoicePaid             = "invoice.paid"
	whStripeCustSubscriptionDeleted = "customer.subscription.deleted"
)

// TODO(pavelb): Gradually replace these everywhere.
type (
	Order     = model.Order
	OrderItem = model.OrderItem
	Issuer    = model.Issuer
)

func decodeAndUnmarshalSku(sku string) (*macaroon.Macaroon, error) {
	macBytes, err := macaroon.Base64Decode([]byte(sku))
	if err != nil {
		return nil, fmt.Errorf("failed to b64 decode sku token: %w", err)
	}
	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sku token: %w", err)
	}

	return mac, nil
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func (s *Service) CreateOrderItemFromMacaroon(ctx context.Context, sku string, quantity int) (*OrderItem, []string, *model.IssuerConfig, error) {
	sublogger := logging.Logger(ctx, "CreateOrderItemFromMacaroon")

	// validation prior to decoding/unmarshalling
	valid, err := validateHardcodedSku(ctx, sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to validate sku")
		return nil, nil, nil, fmt.Errorf("failed to validate sku: %w", err)
	}

	// perform validation
	if !valid {
		sublogger.Error().Err(err).Msg("invalid sku")
		return nil, nil, nil, model.ErrInvalidSKU
	}

	// read the macaroon, its valid
	mac, err := decodeAndUnmarshalSku(sku)
	if err != nil {
		sublogger.Error().Err(err).Msg("failed to decode sku")
		return nil, nil, nil, fmt.Errorf("failed to create order item from macaroon: %w", err)
	}

	caveats := mac.Caveats()
	var allowedPaymentMethods []string
	orderItem := OrderItem{}
	orderItem.Quantity = quantity

	orderItem.Location.String = mac.Location()
	orderItem.Location.Valid = true

	issuerConfig := &model.IssuerConfig{
		Buffer:  defaultBuffer,
		Overlap: defaultOverlap,
	}

	for i := 0; i < len(caveats); i++ {
		caveat := mac.Caveats()[i]
		values := strings.Split(string(caveat.Id), "=")
		key := strings.TrimSpace(values[0])
		value := strings.TrimSpace(strings.Join(values[1:], "="))

		switch key {
		case "sku":
			orderItem.SKU = value
		case "price", "amount":
			orderItem.Price, err = decimal.NewFromString(value)
			if err != nil {
				return nil, nil, nil, err
			}
		case "description":
			orderItem.Description.String = value
			orderItem.Description.Valid = true
		case "currency":
			orderItem.Currency = value
		case "credential_type":
			orderItem.CredentialType = value
		case "issuance_interval":
			orderItem.IssuanceIntervalISO = &value
		case "credential_valid_duration":
			// actually the expires time
			orderItem.ValidFor = new(time.Duration)
			id, err := timeutils.ParseDuration(value)
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			t, err := id.FromNow()
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			*orderItem.ValidFor = time.Until(*t)
			orderItem.ValidForISO = &value
		case "each_credential_valid_duration":
			// for time aware issuers we need to explain per order item
			// what the duration of each credential is
			_, err := timeutils.ParseDuration(value) // parse the duration
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku each_credential_valid_duration")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
			// set the duration iso for the order item
			orderItem.EachCredentialValidForISO = &value
		case "issuer_token_buffer":
			buffer, err := strconv.Atoi(value)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error converting buffer for order item %s: %w", orderItem.ID, err)
			}
			issuerConfig.Buffer = buffer
		case "issuer_token_overlap":
			overlap, err := strconv.Atoi(value)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error converting overlap for order item %s: %w", orderItem.ID, err)
			}
			issuerConfig.Overlap = overlap
		case "allowed_payment_methods":
			allowedPaymentMethods = strings.Split(value, ",")
		case "metadata":
			err := json.Unmarshal([]byte(value), &orderItem.Metadata)
			sublogger.Debug().Str("value", value).Msg("metadata string")
			sublogger.Debug().Str("metadata", fmt.Sprintf("%+v", orderItem.Metadata)).Msg("metadata structure")
			if err != nil {
				sublogger.Error().Err(err).Msg("failed to decode sku metadata")
				return nil, nil, nil, fmt.Errorf("failed to unmarshal macaroon metadata: %w", err)
			}
		}
	}

	newQuantity := decimal.NewFromInt(int64(orderItem.Quantity))
	orderItem.Subtotal = orderItem.Price.Mul(newQuantity)

	return &orderItem, allowedPaymentMethods, issuerConfig, nil
}

func getCustEmailFromStripeCheckout(sess *stripe.CheckoutSession) string {
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

// RenewOrder updates the order status to paid and records payment history.
//
// Status should either be one of pending, paid, fulfilled, or canceled.
func (s *Service) RenewOrder(ctx context.Context, orderID uuid.UUID) error {
	if err := s.Datastore.UpdateOrder(orderID, OrderStatusPaid); err != nil {
		return fmt.Errorf("failed to set order status to paid: %w", err)
	}

	return s.DeleteOrderCreds(ctx, orderID, uuid.Nil, true)
}
