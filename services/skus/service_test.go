package skus

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/test"
	timeutils "github.com/brave-intl/bat-go/libs/time"
	gomock "github.com/golang/mock/gomock"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
)

func TestCredChunkFn(t *testing.T) {
	// Jan 1, 2021
	issued := time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC)

	// 1 day
	day, err := timeutils.ParseDuration("P1D")
	if err != nil {
		t.Errorf("failed to parse 1 day: %s", err.Error())
	}

	// 1 month
	mo, err := timeutils.ParseDuration("P1M")
	if err != nil {
		t.Errorf("failed to parse 1 month: %s", err.Error())
	}

	this, next := credChunkFn(*day)(issued)
	if this.Day() != 20 {
		t.Errorf("day - the next day should be 2")
	}
	if this.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}
	if next.Day() != 21 {
		t.Errorf("day - the next day should be 2")
	}
	if next.Month() != 1 {
		t.Errorf("day - the next month should be 1")
	}

	this, next = credChunkFn(*mo)(issued)
	if this.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if this.Month() != 1 {
		t.Errorf("mo - the next month should be 2")
	}
	if next.Day() != 1 {
		t.Errorf("mo - the next day should be 1")
	}
	if next.Month() != 2 {
		t.Errorf("mo - the next month should be 2")
	}
}

func TestGetTimeLimitedV2Creds_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	order := &Order{
		ID: uuid.NewV4(),
	}

	sor1 := SigningOrderRequest{
		Data: []SigningOrder{
			{
				BlindedTokens: []string{test.RandomString()},
			},
		},
	}

	m1, err := json.Marshal(sor1)
	assert.NoError(t, err)

	outboxMessages := []SigningOrderRequestOutbox{
		{
			Message: m1,
		},
	}

	timeLimitedV2Creds := &TimeLimitedV2Creds{
		Credentials: []TimeAwareSubIssuedCreds{
			{
				BlindedCreds: []string{test.RandomString()},
			},
		},
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetSigningOrderRequestOutbox(ctx, order.ID).
		Return(outboxMessages, nil)

	datastore.EXPECT().
		GetTimeLimitedV2OrderCredsByOrder(order.ID).
		Return(timeLimitedV2Creds, nil)

	s := Service{
		Datastore: datastore,
	}

	actual, status, err := s.GetTimeLimitedV2Creds(ctx, order)
	assert.NoError(t, err)

	assert.Equal(t, timeLimitedV2Creds, actual)
	assert.Equal(t, http.StatusOK, status)
}

func TestGetTimeLimitedV2Creds_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	order := &Order{
		ID: uuid.NewV4(),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetSigningOrderRequestOutbox(ctx, order.ID).
		Return(nil, nil)

	s := Service{
		Datastore: datastore,
	}

	actual, status, err := s.GetTimeLimitedV2Creds(ctx, order)

	assert.Nil(t, actual)
	assert.Equal(t, http.StatusNotFound, status)
	assert.EqualError(t, err, "credentials do not exist")
}

func TestGetTimeLimitedV2Creds_Accepted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	order := &Order{
		ID: uuid.NewV4(),
	}

	sor1 := SigningOrderRequest{
		Data: []SigningOrder{
			{
				BlindedTokens: []string{test.RandomString()},
			},
		},
	}

	m1, err := json.Marshal(sor1)
	assert.NoError(t, err)

	outboxMessages := []SigningOrderRequestOutbox{
		{
			Message: m1,
		},
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetSigningOrderRequestOutbox(ctx, order.ID).
		Return(outboxMessages, nil)

	datastore.EXPECT().
		GetTimeLimitedV2OrderCredsByOrder(order.ID).
		Return(nil, nil)

	s := Service{
		Datastore: datastore,
	}

	orderCreds, status, err := s.GetTimeLimitedV2Creds(ctx, order)
	assert.NoError(t, err)

	assert.Nil(t, orderCreds)
	assert.Equal(t, http.StatusAccepted, status)
}

func TestCalculateTotalExpectedSigningResults(t *testing.T) {
	sor1 := SigningOrderRequest{
		Data: []SigningOrder{
			{
				BlindedTokens: []string{test.RandomString()},
			},
		},
	}

	sor2 := SigningOrderRequest{
		Data: []SigningOrder{
			{
				BlindedTokens: []string{test.RandomString(), test.RandomString()},
			},
		},
	}

	m1, err := json.Marshal(sor1)
	assert.NoError(t, err)

	m2, err := json.Marshal(sor2)
	assert.NoError(t, err)

	outboxMessages := []SigningOrderRequestOutbox{
		{
			Message: m1,
		},
		{
			Message: m2,
		},
	}

	total, err := calculateTotalExpectedSigningResults(outboxMessages)
	assert.NoError(t, err)

	assert.Equal(t, 3, total)
}
