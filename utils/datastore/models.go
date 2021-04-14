package datastore

import (
	"database/sql"
	"encoding/json"
	"strings"
)

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

// FilterColumns filters columns against a list
func FilterColumns(columns, filter []string) []string {
	filterHash := map[string]bool{}
	for _, item := range filter {
		filterHash[item] = true
	}
	filtered := []string{}
	for _, col := range columns {
		if !filterHash[col] {
			filtered = append(filtered, col)
		}
	}
	return filtered
}
