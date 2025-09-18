// Package time parses RFC3339 duration strings into time.Duration
// taken from https://github.com/peterhellberg/duration
package time

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/brave-intl/bat-go/libs/contains"
)

// oneMonth := ISODuration("P1M")
// t, err := oneMonth.InFuture()

// ISODuration - iso representation of
type ISODuration string

// String - implement stringer
func (i *ISODuration) String() string {
	return string(*i)
}

// ParseDuration a RFC3339 duration string into time.Duration
func ParseDuration(s string) (*ISODuration, error) {
	if contains.Str(invalidStrings, s) || strings.HasSuffix(s, "T") {
		return nil, ErrInvalidString
	}

	if !pattern.MatchString(s) {
		return nil, ErrUnsupportedFormat
	}

	d := ISODuration(s)

	return &d, nil
}

// FromNow - add this isoduration to time.Now, resulting in a new time
func (i *ISODuration) FromNow() (*time.Time, error) {
	t, err := i.From(time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to add duration to now: %w", err)
	}
	return t, nil
}

// From returns a new time relative to the ISODuration.
func (i *ISODuration) From(t time.Time) (*time.Time, error) {
	d, err := i.base(t)
	if err != nil {
		return nil, fmt.Errorf("failed to add duration to now: %w", err)
	}

	tt := t.Add(d)
	return &tt, nil
}

const (
	// HoursPerDay is the number of hours per day according to Google
	HoursPerDay = 24.0

	// HoursPerWeek is the number of hours per week according to Google
	HoursPerWeek = 168.0

	// HoursPerMonth is the number of hours per month according to Google
	HoursPerMonth = 730.4841667

	// HoursPerYear is the number of hours per year according to Google
	HoursPerYear = 8765.81
)

var (
	// ErrInvalidString is returned when passed an invalid string
	ErrInvalidString = fmt.Errorf("invalid duration string")

	// ErrUnsupportedFormat is returned when parsing fails
	ErrUnsupportedFormat = fmt.Errorf("unsupported duration string format")

	pattern = regexp.MustCompile(`\A(-)?P((?P<years>[\d\.]+)Y)?((?P<months>[\d\.]+)M)?((?P<weeks>[\d\.]+)W)?((?P<days>[\d\.]+)D)?(T((?P<hours>[\d\.]+)H)?((?P<minutes>[\d\.]+)M)?((?P<seconds>[\d\.]+?)S)?)?\z`)

	invalidStrings = []string{"", "P", "PT"}
)

// base - given a base, produce a time.Duration from base for the ISODuration
func (i *ISODuration) base(t time.Time) (time.Duration, error) {
	if i == nil {
		return 0, nil
	}
	s := i.String()

	var (
		match  []string
		prefix string
	)

	if pattern.MatchString(s) {
		match = pattern.FindStringSubmatch(s)
	} else {
		return 0, ErrUnsupportedFormat
	}

	if strings.HasPrefix(s, "-") {
		prefix = "-"
	}

	return durationFromMatchAndPrefix(match, prefix, t)
}

func durationFunc(prefix string) func(string, float64) time.Duration {
	return func(format string, f float64) time.Duration {
		if d, err := time.ParseDuration(fmt.Sprintf(format, f)); err == nil {
			return d
		}

		return time.Duration(0)
	}
}

func durationFromMatchAndPrefix(match []string, prefix string, t time.Time) (time.Duration, error) {
	d := time.Duration(0)

	duration := durationFunc(prefix)

	for i, name := range pattern.SubexpNames() {
		value := match[i]
		if i == 0 || name == "" || value == "" {
			continue
		}

		if f, err := strconv.ParseFloat(prefix+value, 64); err == nil {
			rem := f - float64(int(f))
			switch name {
			case "years":
				// get actual duration (relative to now)
				d += t.AddDate(int(f), 0, 0).Sub(t)
				if rem > 0 {
					d += duration("%fh", rem*HoursPerYear)
				}
			case "months":
				// get actual duration (relative to now)
				d += t.AddDate(0, int(f), 0).Sub(t)
				if rem > 0 {
					d += duration("%fh", rem*HoursPerMonth)
				}
				//d += duration("%fh", f*HoursPerMonth)
			case "weeks":
				// get actual duration (relative to now)
				d += t.AddDate(0, 0, int(f)*7).Sub(t)
				if rem > 0 {
					d += duration("%fh", rem*HoursPerWeek)
				}
				//d += duration("%fh", f*HoursPerWeek)
			case "days":
				// get actual duration (relative to now)
				d += t.AddDate(0, 0, int(f)).Sub(t)
				//d += duration("%fh", f*HoursPerDay)
				if rem > 0 {
					d += duration("%fh", (f-float64(int(f)))*HoursPerDay)
				}
			case "hours":
				d += duration("%fh", f)
			case "minutes":
				d += duration("%fm", f)
			case "seconds":
				d += duration("%fs", f)
			}
		}
	}

	return d, nil
}
