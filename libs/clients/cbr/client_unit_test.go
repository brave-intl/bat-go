package cbr

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/brave-intl/bat-go/libs/clients"
)

func TestRedeemEquivalence(t *testing.T) {
	tests := []struct {
		name string
		body interface{}
		exp  string
	}{
		{
			name: "not_resp_err_data",
			body: "plain string",
			exp:  "",
		},

		{
			name: "body_not_a_string",
			body: clients.RespErrData{Body: 42},
			exp:  "",
		},

		{
			name: "invalid_json",
			body: clients.RespErrData{Body: "not json"},
			exp:  "",
		},

		{
			name: "no_equivalence_field",
			body: clients.RespErrData{Body: `{"message":"duplicate Redemption"}`},
			exp:  "",
		},

		{
			name: "binding_equivalence",
			body: clients.RespErrData{Body: `{"message":"duplicate Redemption","equivalence":"binding"}`},
			exp:  "binding",
		},

		{
			name: "id_equivalence",
			body: clients.RespErrData{Body: `{"message":"duplicate Redemption","equivalence":"id"}`},
			exp:  "id",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.exp, redeemEquivalence(tc.body))
		})
	}
}
