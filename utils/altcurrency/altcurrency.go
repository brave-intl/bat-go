// Package altcurrency provides an enum-like representation of acceptable cryptocurrencies as
// well as helper functions for tasks like validating addresses and converting currency units.
package altcurrency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/btcsuite/btcutil/base58"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/sha3"
)

// AltCurrency is an enum-like representing a cryptocurrency
// FIXME change this to a struct instead of a type alias?
type AltCurrency int

const (
	invalid AltCurrency = iota
	// BAT Basic Attention Token
	BAT
	// BTC Bitcoin
	BTC
	// ETH Ethereum
	ETH
	// LTC Litecoin
	LTC
)

var altCurrencyName = map[AltCurrency]string{
	BAT: "BAT",
	BTC: "BTC",
	ETH: "ETH",
	LTC: "LTC",
}

var altCurrencyID = map[string]AltCurrency{
	"BAT": BAT,
	"BTC": BTC,
	"ETH": ETH,
	"LTC": LTC,
}

var altCurrencyDecimals = map[AltCurrency]int32{
	BAT: 18,
	BTC: 8,
	ETH: 18,
	LTC: 8,
}

// IsValid returns true if a is a valid AltCurrency.
func (a AltCurrency) IsValid() bool {
	_, exists := altCurrencyName[a]
	if !exists || a == invalid {
		return false
	}
	return true
}

// Scale returns the scalar used to convert between the subunit and the base unit.
// For example in bitcoin this will be 10^8, as there are 10^8 satoshis (subunit)
// in one bitcoin (base unit).
// https://en.wikipedia.org/wiki/Denomination_(currency)#Subunit_and_super_unit
func (a AltCurrency) Scale() decimal.Decimal {
	return decimal.New(1, altCurrencyDecimals[a])
}

// ToProbi converts v, denominated in base units to sub units of AltCurrency a.
func (a AltCurrency) ToProbi(v decimal.Decimal) decimal.Decimal {
	return v.Mul(a.Scale())
}

// FromProbi converts v, denominated in subunits to base units of AltCurrency a.
func (a AltCurrency) FromProbi(v decimal.Decimal) decimal.Decimal {
	return v.DivRound(a.Scale(), altCurrencyDecimals[a])
}

func (a AltCurrency) String() string {
	return altCurrencyName[a]
}

// MarshalText marshalls the altcurrency into text.
func (a *AltCurrency) MarshalText() (text []byte, err error) {
	if *a == invalid {
		return nil, errors.New("not a valid AltCurrency")
	}
	text = []byte(a.String())
	return
}

// UnmarshalText unmarshalls the altcurrency from text.
func (a *AltCurrency) UnmarshalText(text []byte) (err error) {
	*a, err = FromString(string(text))
	return err
}

// FromString returns the corresponding AltCurrency or error if there is none
func FromString(text string) (AltCurrency, error) {
	a, exists := altCurrencyID[text]
	if !exists {
		return invalid, errors.New("not a valid AltCurrency")
	}
	return a, nil
}

// GetBTCAddressVersion returns the BTC address version of the address str.
func GetBTCAddressVersion(str string) int {
	addr := base58.Decode(str)
	if len(addr) != 25 {
		return -1
	}
	version := addr[0]
	checksum := addr[len(addr)-4:]
	vh160 := addr[:len(addr)-4]

	sum := sha256.Sum256(vh160)
	sum = sha256.Sum256(sum[:])
	if !bytes.Equal(sum[0:4], checksum) {
		return -1
	}

	return int(version)
}

// Keccak256 calculates and returns the Keccak256 hash of the input data.
// Copied from https://github.com/ethereum/go-ethereum/, licensed under the GNU General Public License v3.0
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewLegacyKeccak256()
	for _, b := range data {
		_, err := d.Write(b)
		if err != nil {
			panic(err)
		}
	}
	return d.Sum(nil)
}

// ToChecksumETHAddress returns the address str with a checksum encoded in the capitalization per EIP55
func ToChecksumETHAddress(str string) string {
	lower := strings.Replace(strings.ToLower(str), "0x", "", 1)
	lowerBytes := []byte(lower)
	hash := Keccak256([]byte(lower))
	hashHex := make([]byte, hex.EncodedLen(len(hash)))
	hex.Encode(hashHex, hash)

	for i, v := range lowerBytes {
		x, err := strconv.ParseUint(string([]byte{hashHex[i]}), 16, 8)
		if err != nil {
			panic(err)
		}
		if x >= 8 {
			lowerBytes[i] = byte(unicode.ToUpper(rune(v)))
		}
	}
	return "0x" + string(lowerBytes)
}
