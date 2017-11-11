package utils

import (
	"testing"
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
