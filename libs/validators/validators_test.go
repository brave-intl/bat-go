package validators

import (
	"testing"

	"github.com/asaskevich/govalidator"
	uuid "github.com/satori/go.uuid"
)

func TestIsBase64Url(t *testing.T) {
	if !IsBase64Url("eyJ0eXAiOiJKV1QiLA0KICJhbGciOiJIUzI1NiJ9") {
		t.Error("Unexpected error on valid base64url encoded string")
	}
	if IsBase64Url("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk==") {
		t.Error("Unexpected error on valid base64url encoded string with padding")
	}
	if IsBase64Url("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") {
		t.Error("Expected error on base64url encoded string missing padding")
	}
}

func TestIsBase64UrlWithoutPadding(t *testing.T) {
	if !IsBase64UrlWithoutPadding("eyJ0eXAiOiJKV1QiLA0KICJhbGciOiJIUzI1NiJ9") {
		t.Error("Unexpected error on valid base64url encoded string")
	}
	if !IsBase64UrlWithoutPadding("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") {
		t.Error("Unexpected error on base64url encoded string missing padding")
	}
}

func TestIsCompactJWS(t *testing.T) {
	if !IsCompactJWS("eyJ0eXAiOiJKV1QiLA0KICJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJqb2UiLA0KICJleHAiOjEzMDA4MTkzODAsDQogImh0dHA6Ly9leGFtcGxlLmNvbS9pc19yb290Ijp0cnVlfQ.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk") {
		t.Error("Unexpected error on valid compact JWS string")
	}
}

func TestIsBTCAddress(t *testing.T) {
	if !IsBTCAddress("1HZ8g817ZgfLUCALFnnLPdgEUsmwHLb73W") {
		t.Error("Unexpected error on valid BTC address")
	}
	if IsBTCAddress("FHZ8g817ZgfLUCALFnnLPdgEUsmwHLb73W") {
		t.Error("Expected error on valid BTC address")
	}
}

func TestIsETHAddress(t *testing.T) {
	if !IsETHAddress("0xF1A61415e12DB93ABACE8704855A4795934ff992") {
		t.Error("Unexpected error on valid ETH address")
	}
	if IsETHAddress("0xf1a61415e12db93abace8704855a4795934ff992") {
		t.Error("Expected error on ETH address missing checksum")
	}
	if IsETHAddress("0xF1A61415e12DB93ABACE8704855A4795934FF992") {
		t.Error("Unexpected error on ETH address with invalid checksum")
	}
}

func TestIsPlatform(t *testing.T) {
	if IsPlatform("notaplatform") {
		t.Error("non platforms should not pass")
	}
	if IsPlatform("") {
		t.Error("empty strings do not pass")
	}
	if !IsPlatform("osx") {
		t.Error("strings in the list should pass")
	}
}

func TestIsUUID(t *testing.T) {
	if IsUUID("notauuid") {
		t.Error("non uuids should not pass")
	}
	if IsUUID("") {
		t.Error("empty strings do not pass")
	}
	if !IsUUID("01e42e30-a823-4a91-a114-00fd0d47f7d0") {
		t.Error("a uuid should not fail")
	}
	if !IsUUID("424aab2c-3b95-5e7e-9ec3-1ca9349f5887") {
		t.Error("a uuid should not fail")
	}
}

func TestIsEmptyUUID(t *testing.T) {
	type TestRequest struct {
		ID uuid.UUID `valid:"requiredUUID"`
	}

	request := &TestRequest{uuid.FromStringOrNil("01e42e30-a823-4a91-a114-00fd0d47f7d0")}

	isValid, err := govalidator.ValidateStruct(request)
	if err != nil {
		t.Error("should not error")
	}
	if !isValid {
		t.Error("should be valid uuid")
	}

	request.ID = uuid.Nil

	isValid, err = govalidator.ValidateStruct(request)
	if err == nil {
		t.Error("should error", err)
	}
	if isValid {
		t.Error("should not be a valid uuid")
	}

}
