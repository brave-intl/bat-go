package wallet

import (
	"testing"
	"time"

	"github.com/brave-intl/bat-go/libs/ptr"
	should "github.com/stretchr/testify/assert"
)

func TestCustodianLink_isLinked(t *testing.T) {
	type tcGiven struct {
		cl *CustodianLink
	}

	type testCase struct {
		name     string
		given    tcGiven
		expected bool
	}

	tests := []testCase{
		{
			name: "is_linked",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt: time.Now(),
				}},
			expected: true,
		},
		{
			name:  "is_not_linked_nil",
			given: tcGiven{},
		},
		{
			name: "is_not_linked",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt:   time.Now(),
					UnlinkedAt: ptr.To(time.Now()),
				}},
		},
		{
			name: "is_not_linked_disconnected",
			given: tcGiven{
				cl: &CustodianLink{
					LinkedAt:       time.Now(),
					DisconnectedAt: ptr.To(time.Now()),
				}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.cl.isLinked()
			should.Equal(t, tc.expected, actual)
		})
	}
}
