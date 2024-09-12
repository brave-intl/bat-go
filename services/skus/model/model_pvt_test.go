package model

import (
	"errors"
	"testing"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestAddURLParam(t *testing.T) {
	type tcGiven struct {
		src  string
		name string
		val  string
	}

	type tcExpected struct {
		result string
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	// Don't test for invalid inputs due to url.Parse's tolerance.
	tests := []testCase{
		{
			name: "empty",
			exp:  tcExpected{result: "?="},
		},

		{
			name: "add_nothing",
			given: tcGiven{
				src: "https://example.com",
			},
			exp: tcExpected{
				result: "https://example.com?=",
			},
		},

		{
			name: "add_one",
			given: tcGiven{
				src:  "https://example.com",
				name: "param1",
				val:  "val1",
			},
			exp: tcExpected{
				result: "https://example.com?param1=val1",
			},
		},

		{
			name: "add_second",
			given: tcGiven{
				src:  "https://example.com?param=val",
				name: "param2",
				val:  "val2",
			},
			exp: tcExpected{
				result: "https://example.com?param=val&param2=val2",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			act, err := addURLParam(tc.given.src, tc.given.name, tc.given.val)
			must.Equal(t, true, errors.Is(err, tc.exp.err))

			if tc.exp.err != nil {
				return
			}

			should.Equal(t, tc.exp.result, act)
		})
	}
}

func TestFixPremiumSKUForIssuer(t *testing.T) {
	tests := []struct {
		name  string
		given string
		exp   string
	}{
		{
			name: "empty",
		},

		{
			name:  "trimmed_empty",
			given: "-year",
		},

		{
			name:  "untouched",
			given: "anything",
			exp:   "anything",
		},

		{
			name:  "trimmed",
			given: "anything-year",
			exp:   "anything",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := fixPremiumSKUForIssuer(tc.given)
			should.Equal(t, tc.exp, actual)
		})
	}
}
