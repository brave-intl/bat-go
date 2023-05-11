package validators

import (
	"regexp"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/altcurrency"
	uuid "github.com/satori/go.uuid"
)

func init() {
	govalidator.TagMap["base64url"] = govalidator.Validator(IsBase64Url)
	govalidator.TagMap["base64urlnopad"] = govalidator.Validator(IsBase64UrlWithoutPadding)
	govalidator.TagMap["compactjws"] = govalidator.Validator(IsCompactJWS)
	govalidator.TagMap["btcaddress"] = govalidator.Validator(IsBTCAddress)
	govalidator.TagMap["ethaddressnochecksum"] = govalidator.Validator(IsETHAddressNoChecksum)
	govalidator.TagMap["ethaddress"] = govalidator.Validator(IsETHAddress)
	govalidator.TagMap["platform"] = govalidator.Validator(IsPlatform)
	govalidator.CustomTypeTagMap.Set("requiredUUID", govalidator.CustomTypeValidator(IsRequiredUUID))

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

// IsRequiredUUID checks if the uuid is present
func IsRequiredUUID(i interface{}, context interface{}) bool {
	switch v := i.(type) { // you can type switch on the context interface being validated
	case uuid.UUID:
		return !uuid.Equal(v, uuid.Nil)
	default:
		panic("invalid type recieved in IsRequiredUUID")
	}
}

// IsUUID checks if the string is a valid UUID
func IsUUID(v string) bool {
	_, err := uuid.FromString(v)
	return err == nil
}
