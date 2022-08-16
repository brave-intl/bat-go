//go:build integration

package skus

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/skus/skustest"
	"github.com/brave-intl/bat-go/utils/test"
	timeutils "github.com/brave-intl/bat-go/utils/time"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ServiceTestSuite struct {
	suite.Suite
	storage Datastore
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceTestSuite))
}

func (suite *ServiceTestSuite) SetupSuite() {
	skustest.Migrate(suite.T())
	suite.storage, _ = NewPostgres("", false, "")
}

func (suite *ServiceTestSuite) AfterTest() {
	skustest.CleanDB(suite.T(), suite.storage.RawDB())
}

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

	assert.Equal(t, 3, total)
}
