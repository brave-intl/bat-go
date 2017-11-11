package altcurrency

import (
	"encoding/json"
	"github.com/shopspring/decimal"
	"testing"
)

func TestJsonUnmarshal(t *testing.T) {
	var a AltCurrency
	err := json.Unmarshal([]byte("\"BAT\""), &a)
	if err != nil {
		t.Error("Unexpected error during unmarshal")
	}
	if a != BAT {
		t.Error("Unexpected altcurrency to be BAT")
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
	i, _ := decimal.NewFromString("123456789")
	btc := BTC.FromProbi(i)
	expectedBtc, _ := decimal.NewFromString("1.23456789")
	if !btc.Equals(expectedBtc) {
		t.Error("Expected satoshi value to match BTC value")
	}

	i, _ = decimal.NewFromString("1234567898765432123")
	eth := ETH.FromProbi(i)
	expectedEth, _ := decimal.NewFromString("1.234567898765432123")
	if !eth.Equals(expectedEth) {
		t.Error(eth)
		t.Error(expectedEth)
		t.Error("Expected wei value to match ETH value")
	}
}

func TestToProbi(t *testing.T) {
	f, _ := decimal.NewFromString("1.23456789")
	satoshi := BTC.ToProbi(f)
	expectedSatoshi, _ := decimal.NewFromString("123456789")
	if !satoshi.Equals(expectedSatoshi) {
		t.Error("Expected satoshi value to match BTC value")
	}

	f, _ = decimal.NewFromString("1.234567898765432123")
	wei := ETH.ToProbi(f)
	expectedWei, _ := decimal.NewFromString("1234567898765432123")
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
