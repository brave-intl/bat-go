package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
	"strings"
)

// Config defines a GeolocationValidator configuration.
type Config struct {
	bucket string
	object string
}

// GeolocationValidator defines a GeolocationValidator.
type GeolocationValidator struct {
	s3     appaws.S3GetObjectAPI
	config Config
}

// NewGeolocationValidator creates a new instance of NewGeolocationValidator.
func NewGeolocationValidator(s3 appaws.S3GetObjectAPI, config Config) *GeolocationValidator {
	return &GeolocationValidator{
		s3:     s3,
		config: config,
	}
}

// Validate is an implementation of the Validate interface and returns true is a given geolocation is valid.
func (g GeolocationValidator) Validate(ctx context.Context, geolocation string) (bool, error) {
	out, err := g.s3.GetObject(
		ctx, &s3.GetObjectInput{
			Bucket: &g.config.bucket,
			Key:    &g.config.object,
		})
	if err != nil {
		return false, fmt.Errorf("failed to get payout status: %w", err)
	}
	defer func() {
		err := out.Body.Close()
		if err != nil {
			logging.FromContext(ctx).Error().
				Err(err).Msg("error closing body")
		}
	}()

	var locations []string
	err = json.NewDecoder(out.Body).Decode(&locations)
	if err != nil {
		return false, fmt.Errorf("error decoding geolocations")
	}

	for _, location := range locations {
		if strings.EqualFold(location, geolocation) {
			return false, nil
		}
	}

	return true, nil
}
