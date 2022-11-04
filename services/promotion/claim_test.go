package promotion

import (
	"fmt"
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

var suggestionsNeededTests = []struct {
	ApproximateValue  float64
	SuggestionsNeeded int
}{
	{0.1, 1},
	{5.0, 20},
	{5.1, 20},
	{5.124, 20},
	{5.125, 21},
	{5.24, 21},
	{5.25, 21},
}

func TestSuggestionsNeeded(t *testing.T) {
	var claim Claim
	var promotion Promotion

	promotion.ID = uuid.NewV4()
	claim.PromotionID = promotion.ID

	promotion.SuggestionsPerGrant = 40
	promotion.ApproximateValue = decimal.NewFromFloat(10.0)

	for _, tt := range suggestionsNeededTests {
		t.Run(fmt.Sprintf("%f", tt.ApproximateValue), func(t *testing.T) {
			claim.ApproximateValue = decimal.NewFromFloat(tt.ApproximateValue)

			suggestionsNeeded, err := claim.SuggestionsNeeded(&promotion)
			assert.NoError(t, err)

			assert.Equal(t, tt.SuggestionsNeeded, suggestionsNeeded)
		})
	}
}

func TestBlindCredsEq(t *testing.T) {
	var (
		a  = []string{"a", "b", "c"}
		a1 = []string{"a", "b", "c"}
		b  = []string{"b", "a", "c"}
		c  = []string{"d", "b", "c", "a"}
	)
	if blindCredsEq(a, b) {
		t.Error("two creds must retain the same ordering..")
	}
	if blindCredsEq(a, c) {
		t.Error("two creds should have not been equal..")
	}
	if !blindCredsEq(a, a1) {
		t.Error("two creds are equal should not be false")
	}
}

func TestClaimPromotion(t *testing.T) {
	// t.Fatal("not implemented")
}
