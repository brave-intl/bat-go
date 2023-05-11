package datastore

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
)

// Metadata - type which represents key/value pair metadata
type Metadata map[string]interface{}

// Value - implement driver.Valuer interface for conversion to and from sql
func (m Metadata) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan - implement driver.Scanner interface for conversion to and from sql
func (m *Metadata) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan Metadata, not byte slice")
	}
	return json.Unmarshal(b, &m)
}

// NullString is a type that lets ya get a null field from the database
type NullString struct {
	sql.NullString
}

// MarshalJSON for NullString
func (ns *NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// UnmarshalJSON unmarshalls NullString
func (ns *NullString) UnmarshalJSON(data []byte) error {
	ns.String = strings.Trim(string(data), `"`)
	ns.Valid = true
	if len(data) == 0 {
		ns.Valid = false
	}
	return nil
}
