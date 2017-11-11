package utils

import (
	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/altcurrency"
	"regexp"
)

const (
	Base64Url      string = "^(?:[A-Za-z0-9+_-]{4})*(?:[A-Za-z0-9+_-]{2}==|[A-Za-z0-9+_-]{3}=|[A-Za-z0-9+_-]{4})$"
	Base64UrlNoPad string = "^[A-Za-z0-9+_-]+$"
	CompactJWS     string = "^[A-Za-z0-9+_-]+[.][A-Za-z0-9+_-]+[.][A-Za-z0-9+_-]+$"
	BTCAddress     string = "^[a-zA-Z1-9]{27,35}$"
	ETHAddress     string = "^0x[0-9a-fA-F]{40}$"
)

var (
	rxBase64Url      = regexp.MustCompile(Base64Url)
	rxBase64UrlNoPad = regexp.MustCompile(Base64UrlNoPad)
	rxCompactJWS     = regexp.MustCompile(CompactJWS)
	rxBTCAddress     = regexp.MustCompile(BTCAddress)
	rxETHAddress     = regexp.MustCompile(ETHAddress)
)

func IsBase64Url(str string) bool {
	return rxBase64Url.MatchString(str)
}

func IsBase64UrlWithoutPadding(str string) bool {
	return rxBase64UrlNoPad.MatchString(str)
}

func IsCompactJWS(str string) bool {
	return rxCompactJWS.MatchString(str)
}

func IsBTCAddress(str string) bool {
	if !rxBTCAddress.MatchString(str) {
		return false
	}
	if altcurrency.GetBTCAddressVersion(str) < 0 {
		return false
	}
	return true
}

func IsETHAddressNoChecksum(str string) bool {
	return rxETHAddress.MatchString(str)
}

func IsETHAddress(str string) bool {
	if !IsETHAddressNoChecksum(str) {
		return false
	}
	return altcurrency.ToChecksumETHAddress(str) == str
}

func InitValidators() {
	govalidator.SetFieldsRequiredByDefault(true)
	govalidator.TagMap["base64url"] = govalidator.Validator(IsBase64Url)
	govalidator.TagMap["base64urlnopad"] = govalidator.Validator(IsBase64UrlWithoutPadding)
	govalidator.TagMap["compactjws"] = govalidator.Validator(IsCompactJWS)
	govalidator.TagMap["btcaddress"] = govalidator.Validator(IsBTCAddress)
	govalidator.TagMap["ethaddressnochecksum"] = govalidator.Validator(IsETHAddressNoChecksum)
	govalidator.TagMap["ethaddress"] = govalidator.Validator(IsETHAddress)
}
