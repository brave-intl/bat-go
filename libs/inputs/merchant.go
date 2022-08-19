package inputs

import (
	"context"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/handlers"
	uuid "github.com/satori/go.uuid"
)

// MerchantID - identifier of a merchant
type MerchantID struct {
	id uuid.UUID
}

// UUID - get the UUID representation of the merchant id
func (m MerchantID) UUID() uuid.UUID {
	return m.id
}

// NewMerchantID - create a new merchant id
func NewMerchantID(ctx context.Context, v string) (*MerchantID, error) {
	var merchantID = new(MerchantID)
	if err := DecodeAndValidate(ctx, merchantID, []byte(v)); err != nil {
		return nil, handlers.ValidationError(
			"Error decoding or validating request merchant id url parameter",
			map[string]interface{}{
				"merchantID": "merchantID must be a uuidv4",
			},
		)
	}
	return merchantID, nil
}

// Validate - implementation of validatable interface
func (m *MerchantID) Validate(ctx context.Context) error {
	// check that this merchant id is real/exists?
	return nil
}

// Decode - implementation of  decodable interface
func (m *MerchantID) Decode(ctx context.Context, v []byte) error {
	if len(v) == 0 || !govalidator.IsUUIDv4(string(v)) {
		return fmt.Errorf("merchant id is not a uuid: %x", v)
	}

	u, err := uuid.FromString(string(v))
	if err != nil {
		return fmt.Errorf("unable to parse merchant id: %x", v)
	}
	m.id = u
	return nil
}
