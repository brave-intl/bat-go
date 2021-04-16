package stringutils

import (
	"reflect"
	"strings"
)

// SplitAndTrim splits a string along a delimiter and trims it
func SplitAndTrim(base string, options ...string) []string {
	delimiter := ","
	if len(options) > 0 {
		delimiter = options[0]
	}
	split := strings.Split(base, delimiter)
	splitAndTrimmed := []string{}
	for _, s := range split {
		if s == "" {
			continue
		}
		splitAndTrimmed = append(
			splitAndTrimmed,
			strings.TrimSpace(s),
		)
	}
	return splitAndTrimmed
}

// CollectTags collects struct tags
func CollectTags(t interface{}, tags ...string) []string {
	key := "db"
	if len(tags) > 0 {
		key = tags[0]
	}
	reflected := reflect.ValueOf(t).Elem()
	list := []string{}
	for i := 0; i < reflected.NumField(); i++ {
		typeField := reflected.Type().Field(i)
		tag, ok := typeField.Tag.Lookup(key)
		if !ok {
			continue
		}
		splitTag := SplitAndTrim(tag)
		target := splitTag[0]
		if target == "" || target == "-" {
			continue
		}
		list = append(list, target)
	}
	return list
}

// CollectValues collects the values of a struct
func CollectValues(s interface{}) []string {
	list := []string{}
	//
	reflected := reflect.ValueOf(s)
	for i := 0; i < reflected.NumField(); i++ {
		if reflected.Type().Field(i).Type.Kind() == reflect.String {
			list = append(list, reflected.Field(i).String())
		}
	}
	return list
}
