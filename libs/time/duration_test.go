package time_test

import (
	"testing"
	"time"

	should "github.com/stretchr/testify/assert"

	timeutils "github.com/brave-intl/bat-go/libs/time"
)

func TestParse(t *testing.T) {
	for i, tt := range []struct {
		dur string
		err error
		out float64
	}{
		{"PT1.5M", nil, 90},
		{"PT0.5H", nil, 1800},
		{"PT0.5H29M60S", nil, 3600}, // Probably shouldn’t be valid since only the last value can have fractions
		{"PT15S", nil, 15},
		{"PT1M", nil, 60},
		{"PT3M", nil, 180},
		{"PT130S", nil, 130},
		{"PT2M10S", nil, 130},
		{"P1DT2S", nil, 86402},
		{"PT5M10S", nil, 310},
		{"PT1H30M5S", nil, 5405},
		{"P2DT1H10S", nil, 176410},
		{"PT1004199059S", nil, 1004199059},
		{"P3DT5H20M30.123S", nil, 278430.123},
		{"P1W", nil, 604800},
		{"P0.123W", nil, 74390.4},
		{"P1WT5S", nil, 604805},
		{"P1WT1H", nil, 608400},
		//{"P2YT1H30M5S", nil, 63119237}, // constants do not align with new now base
		//{"P1Y2M3DT5H20M30.123S", nil, 37094832.1218}, // constants do not align with new now base
		//{"-P1Y2M3DT5H20M30.123S", nil, -37094832.1218}, // constants do not align with new now base
		{"-P1WT1H", nil, -608400},
		{"-P1DT2S", nil, -86402},
		{"-PT1M5S", nil, -65},
		//{"-P0.123W", nil, -74390.4}, // constants do not align with new now base

		// Not supported since fields in the wrong order
		{"P1M2Y", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since negative value
		{"P-1Y", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since negative value
		{"P1YT-1M", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since missing T
		{"P1S", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since missing P
		{"1Y", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since no value is specified for months
		{"P1YM5D", timeutils.ErrUnsupportedFormat, 0},

		// Not supported since wrong format of string
		{"FOOBAR", timeutils.ErrUnsupportedFormat, 0},

		// Invalid since empty string
		{"", timeutils.ErrInvalidString, 0},

		// Invalid since no time fields present
		{"P", timeutils.ErrInvalidString, 0},

		// Invalid since no time fields present
		{"PT", timeutils.ErrInvalidString, 0},

		// Invalid since ending with T
		{"P1Y2M3DT", timeutils.ErrInvalidString, 0},
	} {

		id, err := timeutils.ParseDuration(tt.dur)
		if err != tt.err {
			t.Fatalf("[%d] unexpected error: %s", i, err)
		}

		n := time.Now()

		ft, err := id.From(n)
		if err != nil {
			t.Fatalf("[%d] unexpected error: %s", i, err)
		}

		d := (*ft).Sub(n)

		if got := d.Seconds(); got != tt.out {
			t.Errorf("[%d] Parse(%q) -> d.Seconds() = %f, want %f", i, tt.dur, got, tt.out)
		}
	}
}

func TestCompareWithTimeParseDuration(t *testing.T) {
	for i, tt := range []struct {
		timeStr     string
		durationStr string
	}{
		{"1h", "PT1H"},
		{"9m60s", "PT10.0M"},
		{"1h2m", "PT1H2M"},
		{"2h15s", "PT1H60M15S"},
		{"169h", "P1WT1H"},
	} {
		td, _ := time.ParseDuration(tt.timeStr)

		id, err := timeutils.ParseDuration(tt.durationStr)
		if err != nil {
			t.Fatalf("[%d] unexpected error: %s", i, err)
		}

		n := time.Now()

		ft, err := id.From(n)
		if err != nil {
			t.Fatalf("[%d] unexpected error: %s", i, err)
		}

		dd := (*ft).Sub(n)

		if td != dd {
			t.Errorf(`[%d] not equal: %q->%v != %q->%v`, i, tt.timeStr, td, tt.durationStr, dd)
		}
	}
}

func TestISODuration_From(t *testing.T) {
	type tcGiven struct {
		now      time.Time
		duration timeutils.ISODuration
	}

	type tcExpected struct {
		date *time.Time
		err  error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "add_one_month",
			given: tcGiven{
				now:      time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
				duration: "P1M",
			},
			exp: tcExpected{
				date: ptrTo(time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC)),
			},
		},

		{
			name: "add_one_year",
			given: tcGiven{
				now:      time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
				duration: "P1Y",
			},
			exp: tcExpected{
				date: ptrTo(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)),
			},
		},

		{
			name: "error_unsupported_format",
			given: tcGiven{
				duration: "unsupported_format",
			},
			exp: tcExpected{
				err: timeutils.ErrUnsupportedFormat,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := tc.given.duration.From(tc.given.now)
			should.ErrorIs(t, err, tc.exp.err)
			should.Equal(t, tc.exp.date, actual)
		})
	}
}

func ptrTo[T any](v T) *T {
	return &v
}
