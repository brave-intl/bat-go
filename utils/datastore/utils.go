package datastore

import (
	"fmt"
	"strings"
)

func JoinStringList(list []string) string {
	return fmt.Sprintf("{%s}", strings.Join(list, ","))
}

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

func ColumnsToParamNames(columns []string) []string {
	return MapStringList(columns, func(item string, index int) string {
		return ":" + item
	})
}
