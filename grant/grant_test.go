package grant

import (
	"sort"
	"testing"
)

func TestByExpiryTimestamp(t *testing.T) {
	grants := []Grant{{ExpiryTimestamp: 12345}, {ExpiryTimestamp: 1234}}
	sort.Sort(ByExpiryTimestamp(grants))
	var last int64
	for _, grant := range grants {
		if grant.ExpiryTimestamp < last {
			t.Error("Order is not correct")
			last = grant.ExpiryTimestamp
		}
	}
}
