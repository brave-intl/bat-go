package digest

import (
	"crypto"
	"testing"
)

func TestMarshalText(t *testing.T) {
	var d Instance
	d.Hash = crypto.SHA256
	d.Digest = "=FOOBAR=="
	b, err := d.MarshalText()
	if err != nil {
		t.Error("Unexpected error during marshal")
	}
	if string(b) != "SHA-256==FOOBAR==" {
		t.Error("Incorrect marshal")
	}
}

func TestUnmarshalText(t *testing.T) {
	var expected, d Instance
	expected.Hash = crypto.SHA256
	expected.Digest = "=FOOBAR=="

	str := "SHA-256==FOOBAR=="
	err := d.UnmarshalText([]byte(str))
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}
	if d != expected {
		t.Error("Incorrect unmarshal")
	}
}

func TestCalculate(t *testing.T) {
	expected := "uU0nuZNNPgilLlLX2n2r+sSE7+N6U4DukIj3rOLvzek="
	var d Instance
	d.Hash = crypto.SHA256
	out := d.Calculate([]byte("hello world"))
	if out != expected {
		t.Error("Incorrect calc")
	}
}

func TestUpdate(t *testing.T) {
	expected := "uU0nuZNNPgilLlLX2n2r+sSE7+N6U4DukIj3rOLvzek="
	var d Instance
	d.Hash = crypto.SHA256
	d.Update([]byte("hello world"))
	if d.Digest != expected {
		t.Error("Incorrect update")
	}
}

func TestVerify(t *testing.T) {
	var d Instance
	err := d.UnmarshalText([]byte("SHA-256=uU0nuZNNPgilLlLX2n2r+sSE7+N6U4DukIj3rOLvzek="))
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}

	if !d.Verify([]byte("hello world")) {
		t.Error("Incorrect calc")
	}

	if d.Verify([]byte("foo bar")) {
		t.Error("Incorrect calc")
	}
}
