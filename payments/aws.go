package payments

import (
	"context"

	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

type awsClient struct{}

// RetrieveSecrets - implements secret discovery for payments service
func (ac *awsClient) RetrieveSecrets(ctx context.Context, uri string) ([]byte, error) {
	return nil, errorutils.ErrNotImplemented
}
