package middleware

import (
	"strings"
	"testing"
)

func Test_isSimpleTokenValid(t *testing.T) {
	tokenList := strings.Split("", ",")
	if isSimpleTokenValid(tokenList, "") {
		t.Error("Expected empty token to always be invalid")
	}

	if isSimpleTokenValid([]string{}, "") {
		t.Error("Expected empty token list to always to be invalid")
	}

	tokenList = strings.Split("FOO", ",")
	if !isSimpleTokenValid(tokenList, "FOO") {
		t.Error("Expected single token to be valid")
	}
	if isSimpleTokenValid(tokenList, "BAR") {
		t.Error("Expected wrong token to be invalid")
	}

	tokenList = strings.Split("FOO ", ",")
	if isSimpleTokenValid(tokenList, "FOO") {
		t.Error("Expected single token to be invalid if list is malformed")
	}

	tokenList = strings.Split("FOO,BAR", ",")
	if !isSimpleTokenValid(tokenList, "FOO") {
		t.Error("Expected multiple tokens to be valid")
	}
	if !isSimpleTokenValid(tokenList, "BAR") {
		t.Error("Expected multiple tokens to be valid")
	}
	if isSimpleTokenValid(tokenList, "XXX") {
		t.Error("Expected wrong tokens to be invalid")
	}
}
