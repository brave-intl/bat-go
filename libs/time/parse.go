package time

import (
	"fmt"
	"time"
)

// ParseStringToTime takes a string pointer and returns a time value formatted as time.RFC3339.
// The provided value must be parsable to time.RFC3339.
// If the pointer value is nil then nil is returned
func ParseStringToTime(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, fmt.Errorf("error parsing value %s", *value)
	}
	return &t, nil
}
