package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	appaws "github.com/brave-intl/bat-go/libs/aws"
	"github.com/brave-intl/bat-go/libs/logging"
)

// Config defines a GeoCountryValidator configuration.
type Config struct {
	bucket string
	object string
}

// GeoCountryValidator defines a GeoCountryValidator.
type GeoCountryValidator struct {
	s3     appaws.S3GetObjectAPI
	config Config
}

// NewGeoCountryValidator creates a new instance of NewGeoCountryValidator.
func NewGeoCountryValidator(s3 appaws.S3GetObjectAPI, config Config) *GeoCountryValidator {
	return &GeoCountryValidator{
		s3:     s3,
		config: config,
	}
}

// Validate is an implementation of the Validate interface and returns true is a given geo country is valid.
func (g GeoCountryValidator) Validate(ctx context.Context, geoCountry string) (bool, error) {
	out, err := g.s3.GetObject(
		ctx, &s3.GetObjectInput{
			Bucket: &g.config.bucket,
			Key:    &g.config.object,
		})
	if err != nil {
		return false, fmt.Errorf("error failed to get s3 object: %w", err)
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
		return false, fmt.Errorf("error decoding geo country s3 list")
	}

	for _, location := range locations {
		if strings.EqualFold(location, geoCountry) {
			return false, nil
		}
	}

	return true, nil
}
