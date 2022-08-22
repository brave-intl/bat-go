package jsonutils

import (
	"database/sql/driver"
	"encoding/json"

	"github.com/jmoiron/sqlx/types"
)

// JSONStringArray is a wrapper around a string array for sql serialization purposes
type JSONStringArray []string

// Scan the src sql type into the passed JSONStringArray
func (arr *JSONStringArray) Scan(src interface{}) error {
	var jt types.JSONText

	if err := jt.Scan(src); err != nil {
		return err
	}

	if err := jt.Unmarshal(arr); err != nil {
		return err
	}

	return nil
}

// Value the driver.Value representation
func (arr *JSONStringArray) Value() (driver.Value, error) {
	var jt types.JSONText

	data, err := json.Marshal((*[]string)(arr))
	if err != nil {
		return nil, err
	}

	if err := jt.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	return jt.Value()
}

// MarshalJSON returns the JSON representation
func (arr *JSONStringArray) MarshalJSON() ([]byte, error) {
	return json.Marshal((*[]string)(arr))
}

// UnmarshalJSON sets the passed JSONStringArray to the value deserialized from JSON
func (arr *JSONStringArray) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, (*[]string)(arr)); err != nil {
		return err
	}

	return nil
}
