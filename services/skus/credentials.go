package skus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	"github.com/brave-intl/bat-go/libs/requestutils"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
)

const (
	defaultMaxTokensPerIssuer = 4000000 // ~1M BAT
	cohort                    = 0
	defaultBuffer             = 30
	defaultOverlap            = 5
)

var ErrOrderUnpaid = errors.New("order not paid")

func decodeIssuerID(issuerID string) (string, string, error) {
	var (
		merchantID string
		sku        string
	)

	u, err := url.Parse(issuerID)
	if err != nil {
		return "", "", fmt.Errorf("parse issuer name: %w", err)
	}

	sku = u.Query().Get("sku")
	u.RawQuery = ""
	merchantID = u.String()

	return merchantID, sku, nil
}

func encodeIssuerID(merchantID, sku string) (string, error) {
	v := url.Values{}
	v.Add("sku", sku)

	u, err := url.Parse(merchantID + "?" + v.Encode())
	if err != nil {
		return "", fmt.Errorf("parse merchant id: %w", err)
	}

	return u.String(), nil
}

// CredentialBinding includes info needed to redeem a single credential
type CredentialBinding struct {
	PublicKey     string `json:"publicKey" valid:"base64"`
	TokenPreimage string `json:"t" valid:"base64"`
	Signature     string `json:"signature" valid:"base64"`
}

// DeduplicateCredentialBindings - given a list of tokens return a deduplicated list
func DeduplicateCredentialBindings(tokens ...CredentialBinding) []CredentialBinding {
	var (
		seen   = map[string]bool{}
		result = []CredentialBinding{}
	)
	for _, t := range tokens {
		if !seen[t.TokenPreimage] {
			seen[t.TokenPreimage] = true
			result = append(result, t)
		}
	}
	return result
}

// Issuer includes information about a particular credential issuer
type Issuer struct {
	ID         uuid.UUID `json:"id" db:"id"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	MerchantID string    `json:"merchantId" db:"merchant_id"`
	PublicKey  string    `json:"publicKey" db:"public_key"`
}

// CreateIssuer creates a new challenge bypass credential issuer, saving it's information into the datastore
func (s *Service) CreateIssuer(ctx context.Context, merchantID string) (*Issuer, error) {
	issuer := &Issuer{MerchantID: merchantID}

	err := s.cbClient.CreateIssuer(ctx, issuer.Name(), defaultMaxTokensPerIssuer)
	if err != nil {
		return nil, err
	}

	resp, err := s.cbClient.GetIssuer(ctx, issuer.Name())
	if err != nil {
		return nil, err
	}

	issuer.PublicKey = resp.PublicKey

	return s.Datastore.InsertIssuer(issuer)
}

// Name returns the name of the issuer as known by the challenge bypass server
func (issuer *Issuer) Name() string {
	return issuer.MerchantID
}

// GetOrCreateIssuer gets a matching issuer if one exists and otherwise creates one
func (s *Service) GetOrCreateIssuer(ctx context.Context, merchantID string) (*Issuer, error) {
	issuer, err := s.Datastore.GetIssuer(merchantID)
	if issuer == nil {
		issuer, err = s.CreateIssuer(ctx, merchantID)
	}

	return issuer, err
}

// OrderCreds encapsulates the credentials to be signed in response to a completed order
type OrderCreds struct {
	ID           uuid.UUID                  `json:"id" db:"item_id"`
	OrderID      uuid.UUID                  `json:"orderId" db:"order_id"`
	IssuerID     uuid.UUID                  `json:"issuerId" db:"issuer_id"`
	BlindedCreds jsonutils.JSONStringArray  `json:"blindedCreds" db:"blinded_creds"`
	SignedCreds  *jsonutils.JSONStringArray `json:"signedCreds" db:"signed_creds"`
	BatchProof   *string                    `json:"batchProof" db:"batch_proof"`
	PublicKey    *string                    `json:"publicKey" db:"public_key"`
}

// TimeLimitedCreds encapsulates time-limited credentials
type TimeLimitedCreds struct {
	ID        uuid.UUID `json:"id"`
	OrderID   uuid.UUID `json:"orderId"`
	IssuedAt  string    `json:"issuedAt"`
	ExpiresAt string    `json:"expiresAt"`
	Token     string    `json:"token"`
}

// TimeLimitedV2Creds encapsulates the credentials to be signed in response to a completed order
type TimeLimitedV2Creds struct {
	OrderID     uuid.UUID                 `json:"orderId"`
	IssuerID    uuid.UUID                 `json:"issuerId" `
	Credentials []TimeAwareSubIssuedCreds `json:"credentials"`
}

// TimeAwareSubIssuedCreds - sub issued time aware credentials
type TimeAwareSubIssuedCreds struct {
	OrderID      uuid.UUID                  `json:"-" db:"order_id"`
	ItemID       uuid.UUID                  `json:"-" db:"item_id"`
	IssuerID     uuid.UUID                  `json:"-" db:"issuer_id"`
	ValidFrom    *string                    `json:"validFrom" db:"valid_from"`
	ValidTo      *string                    `json:"validTo" db:"valid_to"`
	BlindedCreds jsonutils.JSONStringArray  `json:"blindedCreds" db:"blinded_creds"`
	SignedCreds  *jsonutils.JSONStringArray `json:"signedCreds" db:"signed_creds"`
	BatchProof   *string                    `json:"batchProof" db:"batch_proof"`
	PublicKey    *string                    `json:"publicKey" db:"public_key"`
}

// CreateOrderCreds if the order is complete
func (s *Service) CreateOrderCreds(ctx context.Context, orderID uuid.UUID, itemID uuid.UUID, blindedCreds []string) error {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return errorutils.Wrap(err, "error finding order")
	}

	if order == nil {
		return errorutils.Wrap(err, "error no order found")
	}

	if !order.IsPaid() {
		return errors.New("order has not yet been paid")
	}

	// get the order items, need to create issuers based on the
	// special sku values on the order items
	for _, orderItem := range order.Items {
		// generalized issuer based on sku and merchant id
		issuerID, err := encodeIssuerID(order.MerchantID, orderItem.SKU)
		if err != nil {
			return errorutils.Wrap(err, "error encoding issuer name")
		}

		// create the issuer
		issuer, err := s.GetOrCreateIssuer(ctx, issuerID)
		if err != nil {
			return errorutils.Wrap(err, "error finding issuer")
		}

		if len(blindedCreds) > orderItem.Quantity {
			blindedCreds = blindedCreds[:orderItem.Quantity]
		}

		orderCreds := OrderCreds{
			ID:           orderItem.ID,
			OrderID:      orderID,
			IssuerID:     issuer.ID,
			BlindedCreds: jsonutils.JSONStringArray(blindedCreds),
		}

		err = s.Datastore.InsertOrderCreds(&orderCreds)
		if err != nil {
			return errorutils.Wrap(err, "error inserting order creds")
		}
	}

	return nil
}

func (s *Service) CreateOrderCredentials(ctx context.Context, orderID uuid.UUID, itemID uuid.UUID, blindedCreds []string) error {
	order, err := s.Datastore.GetOrder(orderID)
	if err != nil {
		return fmt.Errorf("error get order: %w", err)
	}

	if order == nil {
		return fmt.Errorf("error retrieving orderID %s: %w", orderID, errorutils.ErrNotFound)
	}

	if !order.IsPaid() {
		return ErrOrderUnpaid
	}

	// get the order items, need to create issuers based on the
	// special sku values on the order items
	for _, orderItem := range order.Items {
		// generalized issuer based on sku and merchant id
		issuerID, err := encodeIssuerID(order.MerchantID, orderItem.SKU)
		if err != nil {
			return errorutils.Wrap(err, "error encoding issuer name")
		}

		issuer, err := s.Datastore.GetIssuer(issuerID)
		if err != nil {
			return fmt.Errorf("error getting issuer: %w", err)
		}

		// If no issuer exists for the sku then create a new one
		// This only happens in event of a new sku being created
		if issuer == nil {

			if orderItem.ValidForISO == nil {
				return fmt.Errorf("error duration cannot be nil")
			}

			t := time.Now()

			overlap := defaultOverlap
			if over, ok := orderItem.Metadata["overlap"]; ok {
				overlap, err = strconv.Atoi(over)
				if err != nil {
					return fmt.Errorf("error converting overlap")
				}
			}

			buffer := defaultBuffer
			if buff, ok := orderItem.Metadata["buffer"]; ok {
				buffer, err = strconv.Atoi(buff)
				if err != nil {
					return fmt.Errorf("error converting buffer")
				}
			}

			createIssuerV3 := cbr.CreateIssuerV3{
				Name:      order.MerchantID,
				Cohort:    cohort,
				MaxTokens: defaultMaxTokensPerIssuer,
				ValidFrom: &t, // TODO what these be
				ExpiresAt: &t, // TODO  what should be used
				Duration:  *orderItem.ValidForISO,
				Overlap:   overlap,
				Buffer:    buffer,
			}

			err = s.cbClient.CreateIssuerV3(ctx, createIssuerV3)
			if err != nil {
				return fmt.Errorf("error creating order credentials: error creating issuer v3: %w", err)
			}

			resp, err := s.cbClient.GetIssuer(ctx, createIssuerV3.Name)
			if err != nil {
				return fmt.Errorf("error getting issuer: %w", err)
			}

			issuer, err = s.Datastore.InsertIssuer(&Issuer{
				MerchantID: resp.Name,
				PublicKey:  resp.PublicKey,
			})
			if err != nil {
				return fmt.Errorf("error creating new issuer: %w", err)
			}
		}

		orderCreds := OrderCreds{
			ID:           orderItem.ID,
			OrderID:      orderID,
			IssuerID:     issuer.ID,
			BlindedCreds: jsonutils.JSONStringArray(blindedCreds),
		}

		err = s.Datastore.InsertOrderCreds(&orderCreds)
		if err != nil {
			return fmt.Errorf("error creating order creds: could not insert order creds: %w", err)
		}

		// write to kafka topic for signing

		requestID, ok := ctx.Value(requestutils.RequestID).(string)
		if !ok {
			return errors.New("error retrieving requestID from context for create order credentials")
		}

		associatedData := make(map[string]string)
		associatedData["order_id"] = orderID.String()
		associatedData["item_id"] = itemID.String()

		bytes, err := json.Marshal(associatedData)
		if err != nil {
			return fmt.Errorf("error serializing associated data: %w", err)
		}

		signingOrderRequest := SigningOrderRequest{
			RequestID: requestID,
			Data: []SigningOrder{
				{
					IssuerType:     orderItem.SKU,
					IssuerCohort:   cohort,
					BlindedTokens:  blindedCreds,
					AssociatedData: bytes,
				},
			},
		}

		textual, err := json.Marshal(signingOrderRequest)
		if err != nil {
			return fmt.Errorf("error marshaling kafka msg: %w", err)
		}

		native, _, err := s.codecs[kafkaUnsignedOrderCredsTopic].NativeFromTextual(textual)
		if err != nil {
			return fmt.Errorf("error converting native from textual: %w", err)
		}

		binary, err := s.codecs[kafkaUnsignedOrderCredsTopic].BinaryFromNative(nil, native)
		if err != nil {
			return fmt.Errorf("error converting binary from native: %w", err)
		}

		err = s.kafkaWriter.WriteMessages(ctx, kafka.Message{
			Topic: kafkaUnsignedOrderCredsTopic,
			Key:   []byte(signingOrderRequest.RequestID),
			Value: binary,
		})
		if err != nil {
			return fmt.Errorf("error writting kafka message: %w", err)
		}
	}

	return nil
}

// OrderWorker attempts to work on an order job by signing the blinded credentials of the client
type OrderWorker interface {
	SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error)
}

// SignOrderCreds signs the blinded credentials
func (s *Service) SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error) {

	resp, err := s.cbClient.SignCredentials(ctx, issuer.Name(), blindedCreds)
	if err != nil {
		return nil, err
	}

	signedTokens := jsonutils.JSONStringArray(resp.SignedTokens)

	creds := &OrderCreds{
		ID:           orderID,
		BlindedCreds: blindedCreds,
		SignedCreds:  &signedTokens,
		BatchProof:   &resp.BatchProof,
		PublicKey:    &issuer.PublicKey,
	}

	return creds, nil
}

type OrderCredentialsWorker interface {
	FetchSignedOrderCredentials(ctx context.Context) (*SigningOrderResult, error)
}

func (s *Service) FetchSignedOrderCredentials(ctx context.Context) (*SigningOrderResult, error) {
	message, err := s.kafkaOrderCredsSignedRequestReader.ReadMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("read message: error reading kafka message %w", err)
	}

	codec, ok := s.codecs[kafkaSignedOrderCredsTopic]
	if !ok {
		return nil, fmt.Errorf("read message: could not find codec %s", kafkaSignedOrderCredsTopic)
	}

	native, _, err := codec.NativeFromBinary(message.Value)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode naitve from binary %w", err)
	}

	textual, err := codec.TextualFromNative(nil, native)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode textual from native %w", err)
	}

	var signedOrderResult SigningOrderResult
	err = json.Unmarshal(textual, &signedOrderResult)
	if err != nil {
		return nil, fmt.Errorf("read message: error could not decode json from textual %w", err)
	}

	return &signedOrderResult, nil
}

// generateCredentialRedemptions - helper to create credential redemptions from cred bindings
var generateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
	// deduplicate credential bindings
	cb = DeduplicateCredentialBindings(cb...)

	var (
		requestCredentials = make([]cbr.CredentialRedemption, len(cb))
		issuers            = make(map[string]*Issuer)
	)

	db, ok := ctx.Value(appctx.DatastoreCTXKey).(Datastore)
	if !ok {
		return nil, errors.New("failed to get datastore from context")
	}

	for i := 0; i < len(cb); i++ {

		var (
			ok     bool
			issuer *Issuer
			err    error
		)

		publicKey := cb[i].PublicKey

		if issuer, ok = issuers[publicKey]; !ok {
			issuer, err = db.GetIssuerByPublicKey(publicKey)
			if err != nil {
				return nil, fmt.Errorf("error finding issuer: %w", err)
			}
		}

		requestCredentials[i].Issuer = issuer.Name()
		requestCredentials[i].TokenPreimage = cb[i].TokenPreimage
		requestCredentials[i].Signature = cb[i].Signature
	}
	return requestCredentials, nil
}
