package promotion

import (
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestRedeemedAt(t *testing.T) {
	nullTime := pq.NullTime{}
	solvedTime := pq.NullTime{}
	err := solvedTime.Scan(time.Now().UTC())
	if err != nil {
		t.Error("time scan error")
	}
	grants := []Claim{
		{
			RedeemedAt: nullTime,
		},
		{
			RedeemedAt: solvedTime,
		},
	}
	for idx, grant := range grants {
		var shouldBeSolved bool
		if idx > 0 {
			shouldBeSolved = true
		}
		isRedeemed := grant.IsRedeemed()
		if shouldBeSolved != isRedeemed {
			t.Error("redeemed value does not match")
		}
	}
}
