package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGeoCountryValidator_Validate_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	disabledGeoCountries := []string{
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
	}

	b, err := json.Marshal(disabledGeoCountries)
	assert.NoError(t, err)

	buffer := bytes.NewBuffer(b)
	body := io.NopCloser(buffer)

	out := &s3.GetObjectOutput{
		Body: body,
	}

	s3g := &mockS3Getter{
		fnGetObject: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return out, nil
		},
	}

	config := Config{
		bucket: test.RandomString(),
		object: test.RandomString(),
	}

	g := NewGeoCountryValidator(s3g, config)

	enabled, err := g.Validate(context.Background(), test.RandomString())
	assert.NoError(t, err)

	assert.True(t, enabled)
}

func TestGeoCountryValidator_Validate_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	disabledGeoCountries := []string{
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
	}

	b, err := json.Marshal(disabledGeoCountries)
	assert.NoError(t, err)

	buffer := bytes.NewBuffer(b)
	body := io.NopCloser(buffer)

	out := &s3.GetObjectOutput{
		Body: body,
	}

	s3g := &mockS3Getter{
		fnGetObject: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return out, nil
		},
	}

	config := Config{
		bucket: test.RandomString(),
		object: test.RandomString(),
	}

	g := NewGeoCountryValidator(s3g, config)

	enabled, err := g.Validate(context.Background(), disabledGeoCountries[3])
	assert.NoError(t, err)

	assert.False(t, enabled)
}

type mockS3Getter struct {
	fnGetObject func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (g *mockS3Getter) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if g.fnGetObject == nil {
		return nil, nil
	}

	return g.fnGetObject(ctx, params, optFns...)
}
