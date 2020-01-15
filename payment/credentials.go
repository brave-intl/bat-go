package payment

import (
	"context"
	"time"

	"github.com/brave-intl/bat-go/utils/jsonutils"
	raven "github.com/getsentry/raven-go"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

const (
	defaultMaxTokensPerIssuer = 4000000 // ~1M BAT
)

// Issuer includes information about a particular credential issuer
type Issuer struct {
	ID         uuid.UUID `db:"id"`
	CreatedAt  time.Time `db:"created_at"`
	MerchantID string    `db:"merchant_id"`
	PublicKey  string    `db:"public_key"`
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

	return service.datastore.InsertIssuer(issuer)
}

// Name returns the name of the issuer as known by the challenge bypass server
func (issuer *Issuer) Name() string {
	return issuer.MerchantID
}

// GetOrCreateIssuer gets a matching issuer if one exists and otherwise creates one
func (service *Service) GetOrCreateIssuer(ctx context.Context, merchantID string) (*Issuer, error) {
	issuer, err := service.datastore.GetIssuer(merchantID)
	if err != nil {
		return nil, err
	}

	if issuer == nil {
		issuer, err = service.CreateIssuer(ctx, merchantID)
	}

	return issuer, err
}

// OrderCreds encapsulates the credentials to be signed in response to a completed order
type OrderCreds struct {
	ID           uuid.UUID                  `db:"item_id"`
	OrderID      uuid.UUID                  `db:"order_id"`
	IssuerID     uuid.UUID                  `db:"issuer_id"`
	BlindedCreds jsonutils.JSONStringArray  `db:"blinded_creds"`
	SignedCreds  *jsonutils.JSONStringArray `db:"signed_creds"`
	BatchProof   *string                    `db:"batch_proof"`
	PublicKey    *string                    `db:"public_key"`
}

// CreateOrderCreds if the order is complete
func (service *Service) CreateOrderCreds(ctx context.Context, orderID uuid.UUID, itemID uuid.UUID, blindedCreds []string) error {
	order, err := service.datastore.GetOrder(orderID)
	if err != nil {
		return errors.Wrap(err, "Error finding order")
	}

	if !order.IsPaid() {
		return errors.New("Order has not yet been paid")
	}

	issuer, err := service.GetOrCreateIssuer(ctx, order.MerchantID)
	if err != nil {
		return errors.Wrap(err, "Error finding issuer")
	}

	orderCreds := OrderCreds{
		ID:           itemID,
		OrderID:      orderID,
		IssuerID:     issuer.ID,
		BlindedCreds: jsonutils.JSONStringArray(blindedCreds),
	}

	err = service.datastore.InsertOrderCreds(&orderCreds)
	if err != nil {
		return errors.Wrap(err, "Error inserting order creds")
	}

	go func() {
		_, err := service.RunNextOrderJob(ctx)
		if err != nil {
			raven.CaptureErrorAndWait(err, nil)
		}
	}()

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
