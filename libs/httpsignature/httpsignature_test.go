package httpsignature

import (
	"encoding/hex"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSigningString(t *testing.T) {
	var s signature
	s.Algorithm = ED25519
	s.KeyID = "Test"
	s.Headers = []string{"(request-target)", "host", "date", "cache-control", "x-example"}

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}
	r.Header.Set("Host", "example.org")
	r.Header.Set("Date", "Tue, 07 Jun 2014 20:51:35 GMT")

	// FIXME Check how go parses headers in http server
	//r.Header.Add("X-Example", "Example header\nwith some whitespace.")
	r.Header.Add("X-Example", "Example header with some whitespace.")

	r.Header.Add("Cache-Control", "max-age=60")
	r.Header.Add("Cache-Control", "must-revalidate")

	expected := "(request-target): get /foo\nhost: example.org\ndate: Tue, 07 Jun 2014 20:51:35 GMT\ncache-control: max-age=60, must-revalidate\nx-example: Example header with some whitespace."

	res, err := s.BuildSigningString(r)
	if err != nil {
		t.Error(err)
		t.Error("Unexpected error while building signing string")
	}
	if string(res) != expected {
		t.Error(string(res))
	}

	// TODO add test to cover multiple headers with different capitalization
	// TODO add test covering request uri with query parameters
	// TODO add test covering no headers (date only)
}

func TestSign(t *testing.T) {
	// ED25519 Test
	var privKey Ed25519PrivKey
	privHex := "96aa9ec42242a9a62196281045705196a64e12b15e9160bbb630e38385b82700e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d"
	privKey, err := hex.DecodeString(privHex)
	if err != nil {
		t.Error(err)
	}

	var s signature
	s.Algorithm = ED25519
	s.KeyID = "primary"
	s.Headers = []string{"foo"}

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}
	r.Header.Set("Foo", "bar")

	err = s.SignRequest(privKey, r)
	if err != nil {
		t.Error("Unexpected error while building ED25519 signing string:", err)
	}

	err = s.UnmarshalText([]byte(r.Header.Get("Signature")))
	if err != nil {
		t.Error(err)
	}

	if s.Sig != "RbGSX1MttcKCpCkq9nsPGkdJGUZsAU+0TpiXJYkwde+0ZwxEp9dXO3v17DwyGLXjv385253RdGI7URbrI7J6DQ==" {
		t.Error("Incorrect signature genearted for ED25519")
	}
}

func TestSignRequest(t *testing.T) {
	// ED25519 Test
	var privKey Ed25519PrivKey
	privHex := "96aa9ec42242a9a62196281045705196a64e12b15e9160bbb630e38385b82700e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d"
	privKey, err := hex.DecodeString(privHex)
	if err != nil {
		t.Error(err)
	}

	var sp SignatureParams
	sp.Algorithm = ED25519
	sp.KeyID = "primary"
	sp.Headers = []string{"foo"}

	ps := ParameterizedSignator{
		SignatureParams: sp,
		Signator:        privKey,
	}

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}
	r.Header.Set("Foo", "bar")

	err = ps.SignRequest(r)
	if err != nil {
		t.Error("Unexpected error while building ED25519 signing string:", err)
	}

	var s signature
	err = s.UnmarshalText([]byte(r.Header.Get("Signature")))
	if err != nil {
		t.Error(err)
	}

	if s.Sig != "RbGSX1MttcKCpCkq9nsPGkdJGUZsAU+0TpiXJYkwde+0ZwxEp9dXO3v17DwyGLXjv385253RdGI7URbrI7J6DQ==" {
		t.Error("Incorrect signature genearted for ED25519")
	}

	// HS2019 Test (HMAC-SHA-512)
	var sp2 SignatureParams
	sp2.Algorithm = HS2019
	sp2.KeyID = "secondary"
	sp2.Headers = []string{"(request-target)", "foo"}

	ps2 := ParameterizedSignator{
		SignatureParams: sp2,
		Signator:        HMACKey(privHex),
	}

	r2, reqErr := http.NewRequest("GET", "http://example.org/foo2", nil)
	if reqErr != nil {
		t.Error(reqErr)
	}
	r2.Header.Set("Foo", "bar")

	signErr := ps2.SignRequest(r2)
	if signErr != nil {
		t.Error("Unexpected error while building HS2019 signing string:", signErr)
	}

	var s2 signature
	err = s2.UnmarshalText([]byte(r2.Header.Get("Signature")))
	if err != nil {
		t.Error(err)
	}

	// Value generated using https://dinochiesa.github.io/httpsig/
	if s2.Sig != "q4hNevLfEiHZVCNUCkfxv89YFdpujD3FHfQUQSRnZPmRnakArWlv/KQRsRvmxL9xamS68KePztm1O+CvjIoX1Q==" {
		t.Error("Incorrect signature generated for HS2019")
	}
}

func TestVerify(t *testing.T) {
	var pubKey Ed25519PubKey
	pubKey, err := hex.DecodeString("e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d")
	if err != nil {
		t.Error(err)
	}

	var s signature
	s.Algorithm = ED25519
	s.KeyID = "primary"
	s.Headers = []string{"foo"}
	s.Sig = "RbGSX1MttcKCpCkq9nsPGkdJGUZsAU+0TpiXJYkwde+0ZwxEp9dXO3v17DwyGLXjv385253RdGI7URbrI7J6DQ=="

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}

	r.Header.Set("Foo", "bar")
	r.Header.Set("Signature", `keyId="primary",algorithm="ed25519",headers="digest",signature="`+s.Sig+`"`)

	err = s.VerifyRequest(pubKey, r)
	assert.NoError(t, err)

	s.Sig = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	r.Header.Set("Signature", `keyId="primary",algorithm="ed25519",headers="digest",signature="`+s.Sig+`"`)

	err = s.VerifyRequest(pubKey, r)
	assert.ErrorIs(t, err, ErrBadSignature)

	var hmacVerifier HMACKey = "yyqz64U$eG?eUAp24Pm!Fn!Cn"
	var s2 signature
	s2.Algorithm = HS2019
	s2.KeyID = "secondary"
	s2.Headers = []string{"foo"}
	sig := "3RCLz6TH2I32nj1NY5YaUWDSCNPiKsAVIXjX4merDeNvrGondy7+f3sWQQJWRwEo90FCrthWrrVcgHqqFevS9Q=="

	req, reqErr := http.NewRequest("GET", "http://example.org/foo2", nil)
	if reqErr != nil {
		t.Error(reqErr)
	}

	req.Header.Set("Foo", "bar")
	req.Header.Set("Signature", `keyId="secondary",algorithm="hs2019",headers="digest",signature="`+sig+`"`)

	err = s2.VerifyRequest(hmacVerifier, req)
	assert.NoError(t, err)

	sig = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	req.Header.Set("Signature", `keyId="secondary",algorithm="hs2019",headers="digest",signature="`+sig+`"`)

	err = s2.VerifyRequest(hmacVerifier, req)
	assert.ErrorIs(t, err, ErrBadSignature)
}

func TestVerifyRequest(t *testing.T) {
	var pubKey Ed25519PubKey
	pubKey, err := hex.DecodeString("e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d")
	if err != nil {
		t.Error(err)
	}

	var sp SignatureParams
	sp.Algorithm = ED25519
	sp.KeyID = "primary"
	sp.Headers = []string{"foo"}

	pkv := ParameterizedKeystoreVerifier{
		SignatureParams: sp,
		Keystore:        &StaticKeystore{pubKey},
	}

	sig := "RbGSX1MttcKCpCkq9nsPGkdJGUZsAU+0TpiXJYkwde+0ZwxEp9dXO3v17DwyGLXjv385253RdGI7URbrI7J6DQ=="

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}

	r.Header.Set("Foo", "bar")
	r.Header.Set("Signature", `keyId="primary",algorithm="ed25519",headers="digest",signature="`+sig+`"`)

	_, keyID, err := pkv.VerifyRequest(r)
	if err != nil {
		t.Error("Unexpected error, signature should be valid:", err)
	}
	if keyID != "primary" {
		t.Error("The keyID should match")
	}

	sig = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	r.Header.Set("Signature", `keyId="primary",algorithm="ed25519",headers="digest",signature="`+sig+`"`)

	_, keyID, err = pkv.VerifyRequest(r)
	if err == nil {
		t.Error("Missing expected error, signature should be invalid:", err)
	}
	if keyID == "primary" {
		t.Error("The keyId should not match")
	}

	var hmacVerifier HMACKey = "yyqz64U$eG?eUAp24Pm!Fn!Cn"
	var sp2 SignatureParams
	sp2.Algorithm = HS2019
	sp2.KeyID = "secondary"
	sp2.Headers = []string{"foo"}

	pkv2 := ParameterizedKeystoreVerifier{
		SignatureParams: sp2,
		Keystore:        &StaticKeystore{hmacVerifier},
	}

	sig = "3RCLz6TH2I32nj1NY5YaUWDSCNPiKsAVIXjX4merDeNvrGondy7+f3sWQQJWRwEo90FCrthWrrVcgHqqFevS9Q=="

	req, reqErr := http.NewRequest("GET", "http://example.org/foo2", nil)
	if reqErr != nil {
		t.Error(reqErr)
	}

	req.Header.Set("Foo", "bar")
	req.Header.Set("Signature", `keyId="secondary",algorithm="hs2019",headers="digest",signature="`+sig+`"`)

	_, keyID, err = pkv2.VerifyRequest(req)
	if err != nil {
		t.Error("Unexpected error, signature should be valid:", err)
	}
	if keyID != "secondary" {
		t.Error("The keyId should match")
	}

	sig = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	req.Header.Set("Signature", `keyId="secondary",algorithm="hs2019",headers="digest",signature="`+sig+`"`)

	_, keyID, err = pkv2.VerifyRequest(req)
	if err == nil {
		t.Error("Missing expected error, signature should be invalid:", err)
	}
	if keyID == "secondary" {
		t.Error("The keyId should not match")
	}
}

func TestTextMarshal(t *testing.T) {
	var s signature
	s.Algorithm = ED25519
	s.KeyID = "Test"
	s.Headers = []string{"(request-target)", "host", "date", "content-type", "digest", "content-length"}
	s.Sig = "Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0="

	b, err := s.MarshalText()
	if err != nil {
		t.Error("Unexpected error during marshal")
	}

	expected := "keyId=\"Test\",algorithm=\"ed25519\",headers=\"(request-target) host date content-type digest content-length\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\""

	if string(b) != expected {
		t.Error("Incorrect string value from marshal")
	}

	s.Headers = []string{}

	b, err = s.MarshalText()
	if err != nil {
		t.Error("Unexpected error during marshal")
	}

	expected = "keyId=\"Test\",algorithm=\"ed25519\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\""

	if string(b) != expected {
		t.Error("Incorrect string value from marshal")
	}
}

func TestTextUnmarshal(t *testing.T) {
	var expected signature
	expected.Algorithm = ED25519
	expected.KeyID = "Test"
	expected.Headers = []string{"(request-target)", "host", "date", "content-type", "digest", "content-length"}
	expected.Sig = "Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0="

	marshalled := "Signature keyId=\"Test\",algorithm=\"ed25519\",headers=\"(request-target) host date content-type digest content-length\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\""

	var s signature
	err := s.UnmarshalText([]byte(marshalled))
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}

	if !reflect.DeepEqual(s, expected) {
		t.Error("Incorrect result from unmarshal")
	}

	// Duplicated field
	marshalled = "Signature keyId=\"Foo\",algorithm=\"ed25519\",headers=\"(request-target) host date content-type digest content-length\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\",keyId=\"Test\""

	err = s.UnmarshalText([]byte(marshalled))
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}

	if !reflect.DeepEqual(s, expected) {
		t.Error("Incorrect result from unmarshal")
	}

	// Missing required field
	marshalled = "Signature algorithm=\"ed25519\",headers=\"(request-target) host date content-type digest content-length\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\""

	err = s.UnmarshalText([]byte(marshalled))
	if err == nil {
		t.Error("No error with missing required field keyId")
	}

	// Missing optional field
	marshalled = "Signature keyId=\"Test\",algorithm=\"ed25519\",signature=\"Ef7MlxLXoBovhil3AlyjtBwAL9g4TN3tibLj7uuNB3CROat/9KaeQ4hW2NiJ+pZ6HQEOx9vYZAyi+7cmIkmJszJCut5kQLAwuX+Ms/mUFvpKlSo9StS2bMXDBNjOh4Auj774GFj4gwjS+3NhFeoqyr/MuN6HsEnkvn6zdgfE2i0=\""

	err = s.UnmarshalText([]byte(marshalled))
	if err != nil {
		t.Error("Error with missing optional field headers")
	}
}

func TestSignatureParamsFromRequest(t *testing.T) {
	var privKey Ed25519PrivKey
	privHex := "96aa9ec42242a9a62196281045705196a64e12b15e9160bbb630e38385b82700e7876fd5cc3a228dad634816f4ec4b80a258b2a552467e5d26f30003211bc45d"
	privKey, err := hex.DecodeString(privHex)
	if err != nil {
		t.Error(err)
	}

	var s signature
	s.Algorithm = ED25519
	s.KeyID = "primary"
	s.Headers = []string{"foo"}

	r, err := http.NewRequest("GET", "http://example.org/foo", nil)
	if err != nil {
		t.Error(err)
	}
	r.Header.Set("Foo", "bar")

	err = s.SignRequest(privKey, r)
	if err != nil {
		t.Error("Unexpected error while building ED25519 signing string:", err)
	}

	sp, err := SignatureParamsFromRequest(r)
	if err != nil {
		t.Error("Unexpected error while retrieving signature params:", err)
	}
	if !reflect.DeepEqual(*sp, s.SignatureParams) {
		t.Error("signature params should match!")
	}

	s.Algorithm = HS2019
	if reflect.DeepEqual(*sp, s.SignatureParams) {
		t.Error("signature params should not match!")
	}
}
