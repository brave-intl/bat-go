package promotion

import (
	"context"

	uuid "github.com/satori/go.uuid"
)

const (
	defaultMaxTokensPerIssuer = 4000000 // ~1M BAT
)

// Issuer includes information about a particular credential issuer
type Issuer struct {
	ID          uuid.UUID `db:"id"`
	PromotionID uuid.UUID `db:"promotion_id"`
	Cohort      string
	PublicKey   string `db:"public_key"`
}

// CreateIssuer creates a new challenge bypass credential issuer, saving it's information into the datastore
func (service *Service) CreateIssuer(ctx context.Context, promotionID uuid.UUID, cohort string) (*Issuer, error) {
	issuer := &Issuer{PromotionID: promotionID, Cohort: cohort, PublicKey: ""}

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
	return issuer.PromotionID.String() + ":" + issuer.Cohort
}

// GetOrCreateIssuer gets a matching issuer if one exists and otherwise creates one
func (service *Service) GetOrCreateIssuer(ctx context.Context, promotionID uuid.UUID, cohort string) (*Issuer, error) {
	issuer, err := service.Datastore.GetIssuer(promotionID, cohort)
	if err != nil {
		return nil, err
	}

	if issuer == nil {
		issuer, err = service.CreateIssuer(ctx, promotionID, cohort)
	}

	return issuer, err
}
