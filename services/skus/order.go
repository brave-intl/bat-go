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
	paymentProcessor = "paymentProcessor"
	// IOSPaymentMethod - indicating this used an ios payment method
	IOSPaymentMethod = "ios"
	// AndroidPaymentMethod - indicating this used an android payment method
	AndroidPaymentMethod = "android"
)

const (
	// TODO(pavelb): Gradually replace it everywhere.
	StripePaymentMethod = model.StripePaymentMethod

	StripeInvoiceUpdated              = "invoice.updated"
	StripeInvoicePaid                 = "invoice.paid"
	StripeCustomerSubscriptionDeleted = "customer.subscription.deleted"
)

// TODO(pavelb): Gradually replace it everywhere.

type Methods = model.Methods

type Order = model.Order

type OrderItem = model.OrderItem

type CreateCheckoutSessionResponse = model.CreateCheckoutSessionResponse

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

// IssuerConfig - the configuration of an issuer
type IssuerConfig struct {
	buffer  int
	overlap int
}

// CreateOrderItemFromMacaroon creates an order item from a macaroon
func (s *Service) CreateOrderItemFromMacaroon(ctx context.Context, sku string, quantity int) (*OrderItem, *Methods, *IssuerConfig, error) {
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
	allowedPaymentMethods := new(Methods)
	orderItem := OrderItem{}
	orderItem.Quantity = quantity

	orderItem.Location.String = mac.Location()
	orderItem.Location.Valid = true

	issuerConfig := &IssuerConfig{
		buffer:  defaultBuffer,
		overlap: defaultOverlap,
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
			if err != nil {
				return nil, nil, nil, err
			}
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
			issuerConfig.buffer = buffer
		case "issuer_token_overlap":
			overlap, err := strconv.Atoi(value)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("error converting overlap for order item %s: %w", orderItem.ID, err)
			}
			issuerConfig.overlap = overlap
		case "allowed_payment_methods":
			*allowedPaymentMethods = Methods(strings.Split(value, ","))
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
	newQuantity, err := decimal.NewFromString(strconv.Itoa(orderItem.Quantity))
	if err != nil {
		return nil, nil, nil, err
	}

	orderItem.Subtotal = orderItem.Price.Mul(newQuantity)

	return &orderItem, allowedPaymentMethods, issuerConfig, nil
}

func getEmailFromCheckoutSession(stripeSession *stripe.CheckoutSession) string {
	// has an existing checkout session
	var email string
	if stripeSession == nil {
		// stripe session does not exist
		return email
	}
	if stripeSession.CustomerEmail != "" {
		// if the email was stored on the stripe session customer email, use it
		email = stripeSession.CustomerEmail
	} else if stripeSession.Customer != nil && stripeSession.Customer.Email != "" {
		// if the stripe session has a customer record, with an email, use it
		email = stripeSession.Customer.Email
	}
	// if there is no record of an email, stripe will ask for it and make a new customer
	return email
}

// RenewOrder updates the orders status to paid and paid at time, inserts record of this order
// Status should either be one of pending, paid, fulfilled, or canceled.
func (s *Service) RenewOrder(ctx context.Context, orderID uuid.UUID) error {

	// renew order is an update order with paid status
	// and an update order expires at with the new expiry time of the order
	err := s.Datastore.UpdateOrder(orderID, OrderStatusPaid) // this performs a record order payment
	if err != nil {
		return fmt.Errorf("failed to set order status to paid: %w", err)
	}

	return s.DeleteOrderCreds(ctx, orderID, true)
}
