package altcurrency

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

func TestJsonUnmarshal(t *testing.T) {
	var a AltCurrency
	err := json.Unmarshal([]byte("\"BAT\""), &a)
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}
	if a != BAT {
		t.Error("Unexpected altcurrency to be BAT")
		t.Error(a)
	}

	err = json.Unmarshal([]byte("\"FOO\""), &a)
	if err == nil {
		t.Error("Expected error during unmarshal")
	}

	err = json.Unmarshal([]byte("\"INVALID\""), &a)
	if err == nil {
		t.Error("Expected error during unmarshal")
	}
}

func TestJsonMarshal(t *testing.T) {
	var a AltCurrency
	_, err := json.Marshal(&a)
	if err == nil {
		t.Error("Expected error during marshal of uninitialized altcurrency")
	}
	a = ETH
	b, err := json.Marshal(&a)
	if err != nil {
		t.Error("Unexpected error during marshal")
	}
	if string(b) != "\"ETH\"" {
		t.Error("Incorrect string value from marshal")
	}
}

func TestFromProbi(t *testing.T) {
	i, err := decimal.NewFromString("123456789")
	if err != nil {
		t.Error(err)
	}
	btc := BTC.FromProbi(i)
	expectedBtc, err := decimal.NewFromString("1.23456789")
	if err != nil {
		t.Error(err)
	}
	if !btc.Equals(expectedBtc) {
		t.Error("Expected satoshi value to match BTC value")
	}

	i, err = decimal.NewFromString("1234567898765432123")
	if err != nil {
		t.Error(err)
	}

	eth := ETH.FromProbi(i)
	expectedEth, err := decimal.NewFromString("1.234567898765432123")
	if err != nil {
		t.Error(err)
	}

	if !eth.Equals(expectedEth) {
		t.Error(eth)
		t.Error(expectedEth)
		t.Error("Expected wei value to match ETH value")
	}
}

func TestToProbi(t *testing.T) {
	f, err := decimal.NewFromString("1.23456789")
	if err != nil {
		t.Error(err)
	}
	satoshi := BTC.ToProbi(f)
	expectedSatoshi, err := decimal.NewFromString("123456789")
	if err != nil {
		t.Error(err)
	}

	if !satoshi.Equals(expectedSatoshi) {
		t.Error("Expected satoshi value to match BTC value")
	}

	f, err = decimal.NewFromString("1.234567898765432123")
	if err != nil {
		t.Error(err)
	}

	wei := ETH.ToProbi(f)
	expectedWei, err := decimal.NewFromString("1234567898765432123")
	if err != nil {
		t.Error(err)
	}

	if !wei.Equals(expectedWei) {
		t.Error("Expected wei value to match ETH value")
	}
}

func TestToChecksumETHAddress(t *testing.T) {
	addr := ToChecksumETHAddress("0xf1a61415e12db93abace8704855a4795934ff992")
	if addr != "0xF1A61415e12DB93ABACE8704855A4795934ff992" {
		t.Error("Unexpected adding checksum to ETH address")
	}
}
