package avro

import (
	"reflect"
	"time"

	"github.com/brave-intl/bat-go/utils/altcurrency"
	stringutils "github.com/brave-intl/bat-go/utils/string"
	"github.com/shopspring/decimal"
)

var (
	interfaceType = reflect.ValueOf([]interface{}{}).Type()
)

// ToNativeSlice converts a value into a native ([]map[string]interface{}) slice
func ToNativeSlice(value interface{}, tagNames ...string) interface{} {
	reflected := reflect.ValueOf(value)
	slice := reflect.MakeSlice(
		interfaceType,
		reflected.Len(),
		reflected.Cap(),
	)
	for i := 0; i < reflected.Len(); i++ {
		value := reflected.Index(i).Interface()
		native := ToNative(value, tagNames...)
		slice.Index(i).Set(
			reflect.ValueOf(native),
		)
	}
	return slice.Interface()
}

// ToNative produces a map[string]interface{}
func ToNative(value interface{}, tagNames ...string) interface{} {
	reflected := reflect.ValueOf(value)
	// add basic custom interfaces here
	switch val := value.(type) {
	case decimal.Decimal:
		return val.String()
	case time.Time:
		return val.Format(time.RFC3339)
	case altcurrency.AltCurrency:
		return val.String()
	}
	// check for basic types
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.UnsafePointer:
		return nil
	case reflect.String:
		return reflected.String()
	case reflect.Int:
		return reflected.Int()
	case reflect.Uint:
		return reflected.Uint()
	case reflect.Bool:
		return reflected.Bool()
	case reflect.Slice:
		return ToNativeSlice(value, tagNames...)
	}
	// dealing with a struct
	m := map[string]interface{}{}
	name := "json"
	if len(tagNames) > 0 {
		name = tagNames[0]
	}
	for i := 0; i < reflected.NumField(); i++ {
		typeField := reflected.Type().Field(i)
		tag, ok := typeField.Tag.Lookup(name)
		if !ok {
			continue
		}
		splitTag := stringutils.SplitAndTrim(tag)
		target := splitTag[0]
		if target == "" || target == "-" {
			continue
		}
		val := ToNative(reflected.Field(i).Interface(), tagNames...)
		if val == nil && shouldOmitEmpty(splitTag[1:]...) {
			continue
		}
		m[target] = val
	}
	return m
}

func shouldOmitEmpty(tags ...string) bool {
	for _, tag := range tags {
		if tag == "omitempty" {
			return true
		}
	}
	return false
}
