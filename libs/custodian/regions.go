package custodian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/brave-intl/bat-go/libs/ptr"
)

type s3Getter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func ExtractCustodianRegions(ctx context.Context, s3g s3Getter, bucket string) (*Regions, error) {
	out, err := s3g.GetObject(
		ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    ptr.To("custodian-regions.json"),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get custodian regions: %w", err)
	}
	defer func() {
		if err := out.Body.Close(); err != nil {
			l := logging.Logger(ctx, "custodian.ExtractCustodianRegions")

			l.Error().Err(err).Msg("failed to close s3 result body")
		}
	}()

	var custodianRegions = new(Regions)

	if err := inputs.DecodeAndValidateReader(ctx, custodianRegions, out.Body); err != nil {
		return nil, custodianRegions.HandleErrors(err)
	}

	return custodianRegions, nil
}

// ExtractPayoutStatus - extract the custodian payout status from the client
func ExtractPayoutStatus(ctx context.Context, s3g s3Getter, bucket string) (*PayoutStatus, error) {
	logger := logging.Logger(ctx, "custodian.extractPayoutStatus")
	// get the object with the client
	out, err := s3g.GetObject(
		ctx, &s3.GetObjectInput{
			Bucket: &bucket,
			Key:    ptr.To("payout-status.json"),
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get payout status: %w", err)
	}
	defer func() {
		if err := out.Body.Close(); err != nil {
			logger.Error().Err(err).Msg("failed to close s3 result body")
		}
	}()
	var payoutStatus = new(PayoutStatus)

	// parse body json
	if err := inputs.DecodeAndValidateReader(ctx, payoutStatus, out.Body); err != nil {
		return nil, payoutStatus.HandleErrors(err)
	}

	return payoutStatus, nil
}

// HandleErrors - handle any errors in input
func (ps *PayoutStatus) HandleErrors(err error) *handlers.AppError {
	return handlers.ValidationError("invalid payout status", err)
}

// Decode - implement decodable
func (ps *PayoutStatus) Decode(ctx context.Context, input []byte) error {
	return json.Unmarshal(input, ps)
}

// Validate - implement validatable
func (ps *PayoutStatus) Validate(ctx context.Context) error {
	isValid, err := govalidator.ValidateStruct(ps)
	if err != nil {
		return err
	}
	if !isValid {
		return errors.New("invalid input")
	}
	return nil
}

// PayoutStatus - current state of the payout status
type PayoutStatus struct {
	Unverified string `json:"unverified" valid:"in(off|processing|complete)"`
	Uphold     string `json:"uphold" valid:"in(off|processing|complete)"`
	Gemini     string `json:"gemini" valid:"in(off|processing|complete)"`
	Bitflyer   string `json:"bitflyer" valid:"in(off|processing|complete)"`
	Zebpay     string `json:"zebpay" valid:"in(off|processing|complete)"`
	Solana     string `json:"solana" valid:"in(off|processing|complete)"`
	Date       string `json:"payoutDate" valid:"-"`
}

// GeoAllowBlockMap - this is the allow / block list of geos for a custodian
type GeoAllowBlockMap struct {
	Allow []string `json:"allow" valid:"-"`
	Block []string `json:"block" valid:"-"`
}

// check if passed in countries exist in an allow or block list
func contains(countries, allowblock []string) bool {
	for _, ab := range allowblock {
		for _, country := range countries {
			if strings.EqualFold(ab, country) {
				return true
			}
		}
	}
	return false
}

// Verdict - test is countries exist in allow list, or do not exist in block list
func (gabm GeoAllowBlockMap) Verdict(countries ...string) bool {
	if len(gabm.Allow) > 0 {
		// allow list exists, use it to check if any countries exist in allow
		return contains(countries, gabm.Allow)
	}
	// check if any block list countries exist in our list of countries
	return !contains(gabm.Block, countries)
}

// Regions - Supported Regions
type Regions struct {
	Uphold   GeoAllowBlockMap `json:"uphold" valid:"-"`
	Gemini   GeoAllowBlockMap `json:"gemini" valid:"-"`
	Bitflyer GeoAllowBlockMap `json:"bitflyer" valid:"-"`
	Zebpay   GeoAllowBlockMap `json:"zebpay" valid:"-"`
	Solana   GeoAllowBlockMap `json:"solana" valid:"-"`
}

// HandleErrors - handle any errors in input
func (cr *Regions) HandleErrors(err error) *handlers.AppError {
	return handlers.ValidationError("invalid custodian regions", err)
}

// Decode - implement decodable
func (cr *Regions) Decode(_ context.Context, input []byte) error {
	return json.Unmarshal(input, cr)
}

// Validate - implement validatable
func (cr *Regions) Validate(_ context.Context) error {
	isValid, err := govalidator.ValidateStruct(cr)
	if err != nil {
		return err
	}
	if !isValid {
		return errors.New("invalid input")
	}
	return nil
}
