package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	mockaws "github.com/brave-intl/bat-go/libs/aws/mock"
	"github.com/brave-intl/bat-go/libs/test"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestGeolocationValidator_Validate_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	disabledGeolocations := []string{
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
	}

	b, err := json.Marshal(disabledGeolocations)
	assert.NoError(t, err)

	buffer := bytes.NewBuffer(b)
	body := io.NopCloser(buffer)

	out := &s3.GetObjectOutput{
		Body: body,
	}

	api := mockaws.NewMockS3GetObjectAPI(ctrl)
	api.EXPECT().GetObject(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(out, nil)

	config := Config{
		bucket: test.RandomString(),
		object: test.RandomString(),
	}

	g := NewGeolocationValidator(api, config)

	enabled, err := g.Validate(context.Background(), test.RandomString())
	assert.NoError(t, err)

	assert.True(t, enabled)
}

func TestGeolocationValidator_Validate_Disabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	disabledGeolocations := []string{
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
		test.RandomString(),
	}

	b, err := json.Marshal(disabledGeolocations)
	assert.NoError(t, err)

	buffer := bytes.NewBuffer(b)
	body := io.NopCloser(buffer)

	out := &s3.GetObjectOutput{
		Body: body,
	}

	api := mockaws.NewMockS3GetObjectAPI(ctrl)
	api.EXPECT().GetObject(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(out, nil)

	config := Config{
		bucket: test.RandomString(),
		object: test.RandomString(),
	}

	g := NewGeolocationValidator(api, config)

	enabled, err := g.Validate(context.Background(), disabledGeolocations[3])
	assert.NoError(t, err)

	assert.False(t, enabled)
}
