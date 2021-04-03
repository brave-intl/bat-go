package datastore

import (
	"fmt"
	"strings"
)

// JoinStringList joins a list of strings to be used as a text array in a query
func JoinStringList(list []string) string {
	return fmt.Sprintf("{%s}", strings.Join(list, ","))
}

// MapStringList maps a list of strings to another string
func MapStringList(
	list []string,
	fn func(string, int) string,
) []string {
	l2 := []string{}
	for i, item := range list {
		l2 = append(l2, fn(item, i))
	}
	return l2
}

// ColumnsToParamNames converts a list of columns to their appropriate named parameter values
func ColumnsToParamNames(columns []string) []string {
	return MapStringList(columns, func(item string, index int) string {
		return ":" + item
	})
}
