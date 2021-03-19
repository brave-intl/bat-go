package validators

import (
	"regexp"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	uuid "github.com/satori/go.uuid"
)

func init() {
	govalidator.CustomTypeTagMap.Set("base64url", stringParseValidator(IsBase64Url))
	govalidator.CustomTypeTagMap.Set("base64urlnopad", stringParseValidator(IsBase64UrlWithoutPadding))
	govalidator.CustomTypeTagMap.Set("compactjws", stringParseValidator(IsCompactJWS))
	govalidator.CustomTypeTagMap.Set("btcaddress", stringParseValidator(IsBTCAddress))
	govalidator.CustomTypeTagMap.Set("ethaddressnochecksum", stringParseValidator(IsETHAddressNoChecksum))
	govalidator.CustomTypeTagMap.Set("ethaddress", stringParseValidator(IsETHAddress))
	govalidator.CustomTypeTagMap.Set("platform", stringParseValidator(IsPlatform))
	govalidator.CustomTypeTagMap.Set("uuid", IsRequiredUUID)
	govalidator.CustomTypeTagMap.Set("uuids", IsRequiredUUIDs)
}

const (
	base64Url      string = "^(?:[A-Za-z0-9+_-]{4})*(?:[A-Za-z0-9+_-]{2}==|[A-Za-z0-9+_-]{3}=|[A-Za-z0-9+_-]{4})$"
	base64UrlNoPad string = "^[A-Za-z0-9+_-]+$"
	compactJWS     string = "^[A-Za-z0-9+_-]+[.][A-Za-z0-9+_-]+[.][A-Za-z0-9+_-]+$"
	btcAddress     string = "^[a-zA-Z1-9]{27,35}$"
	ethAddress     string = "^0x[0-9a-fA-F]{40}$"
)

var (
	rxBase64Url      = regexp.MustCompile(base64Url)
	rxBase64UrlNoPad = regexp.MustCompile(base64UrlNoPad)
	rxCompactJWS     = regexp.MustCompile(compactJWS)
	rxBTCAddress     = regexp.MustCompile(btcAddress)
	rxETHAddress     = regexp.MustCompile(ethAddress)
)

func stringParseValidator(fn func(string) bool) func(interface{}, interface{}) bool {
	return func(i interface{}, context interface{}) bool {
		switch v := i.(type) {
		case string:
			return fn(v)
		}
		return false
	}
}

// IsBase64Url returns true if the string str is base64url (encoded with the "URL and Filename safe" alphabet)
// https://tools.ietf.org/html/rfc4648#section-5
func IsBase64Url(str string) bool {
	return rxBase64Url.MatchString(str)
}

// IsBase64UrlWithoutPadding returns true if the string str is base64url encoded with end padding omitted
func IsBase64UrlWithoutPadding(str string) bool {
	return rxBase64UrlNoPad.MatchString(str)
}

// IsCompactJWS returns true if the string str is a JSW in the compact JSON serialization
func IsCompactJWS(str string) bool {
	return rxCompactJWS.MatchString(str)
}

// IsBTCAddress returns true if the string str is a bitcoin address
func IsBTCAddress(str string) bool {
	if !rxBTCAddress.MatchString(str) {
		return false
	}
	if altcurrency.GetBTCAddressVersion(str) < 0 {
		return false
	}
	return true
}

// IsETHAddressNoChecksum returns true if the string str is a ethereum address
func IsETHAddressNoChecksum(str string) bool {
	return rxETHAddress.MatchString(str)
}

// IsETHAddress returns true if the string str is a ethereum address
func IsETHAddress(str string) bool {
	if !IsETHAddressNoChecksum(str) {
		return false
	}
	return altcurrency.ToChecksumETHAddress(str) == str
}

// IsPlatform determines whether or not a given string is a recognized platform
func IsPlatform(platform string) bool {
	platforms := []string{"ios", "android", "osx", "windows", "linux", "desktop"}
	return govalidator.IsIn(platform, platforms...)
}

func IsRequiredUUID(i interface{}, context interface{}) bool {
	switch v := i.(type) {
	case uuid.UUID:
		return !isUUIDNil(v)
	}
	return false
}

func IsRequiredUUIDs(i interface{}, context interface{}) bool {
	switch v := i.(type) {
	case []uuid.UUID:
		if len(v) == 0 {
			return true
		}
		base := []interface{}{}
		for _, id := range v {
			base = append(base, id)
		}
		found := govalidator.Find(base, func(id_ interface{}, i int) bool {
			return isUUIDNil(id_.(uuid.UUID))
		})
		return found == nil
	}
	return false
}

func isUUIDNil(id uuid.UUID) bool {
	return uuid.Equal(id, uuid.Nil) || uuid.Equal(id, uuid.UUID{})
}

// IsUUID checks if the string is a valid UUID
func IsUUID(v string) bool {
	_, err := uuid.FromString(v)
	return err == nil
}
