package skus

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/jsonutils"
	uuid "github.com/satori/go.uuid"
)

const (
	defaultMaxTokensPerIssuer = 4000000 // ~1M BAT
)

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
func (service *Service) CreateIssuer(ctx context.Context, merchantID string) (*Issuer, error) {
	issuer := &Issuer{MerchantID: merchantID}

	err := service.cbClient.CreateIssuer(ctx, issuer.Name(), defaultMaxTokensPerIssuer)
	if err != nil {
		return nil, err
	}

	resp, err := service.cbClient.GetIssuer(ctx, issuer.Name())
	if err != nil {
		return nil, err
	}

	issuer.PublicKey = resp.PublicKey

	return service.Datastore.InsertIssuer(issuer)
}

// Name returns the name of the issuer as known by the challenge bypass server
func (issuer *Issuer) Name() string {
	return issuer.MerchantID
}

// GetOrCreateIssuer gets a matching issuer if one exists and otherwise creates one
func (service *Service) GetOrCreateIssuer(ctx context.Context, merchantID string) (*Issuer, error) {
	issuer, err := service.Datastore.GetIssuer(merchantID)
	if issuer == nil {
		issuer, err = service.CreateIssuer(ctx, merchantID)
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

// CreateOrderCreds if the order is complete
func (service *Service) CreateOrderCreds(ctx context.Context, orderID uuid.UUID, itemID uuid.UUID, blindedCreds []string) error {
	order, err := service.Datastore.GetOrder(orderID)
	if err != nil {
		return errorutils.Wrap(err, "error finding order")
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
		issuer, err := service.GetOrCreateIssuer(ctx, issuerID)
		if err != nil {
			return errorutils.Wrap(err, "error finding issuer")
		}

		if len(blindedCreds) > orderItem.Quantity {
			blindedCreds = blindedCreds[:orderItem.Quantity]
		}

		orderCreds := OrderCreds{
			ID:           itemID,
			OrderID:      orderID,
			IssuerID:     issuer.ID,
			BlindedCreds: jsonutils.JSONStringArray(blindedCreds),
		}

		err = service.Datastore.InsertOrderCreds(&orderCreds)
		if err != nil {
			return errorutils.Wrap(err, "error inserting order creds")
		}
	}

	return nil
}

// OrderWorker attempts to work on an order job by signing the blinded credentials of the client
type OrderWorker interface {
	SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error)
}

// SignOrderCreds signs the blinded credentials
func (service *Service) SignOrderCreds(ctx context.Context, orderID uuid.UUID, issuer Issuer, blindedCreds []string) (*OrderCreds, error) {

	resp, err := service.cbClient.SignCredentials(ctx, issuer.Name(), blindedCreds)
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
