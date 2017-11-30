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
	"github.com/ethereum/go-ethereum/crypto/sha3"
	"github.com/shopspring/decimal"
)

// FIXME change this to a struct instead of a type alias?
type AltCurrency int

const (
	INVALID AltCurrency = iota
	BAT
	BTC
	ETH
	LTC
)

var altCurrencyName = map[AltCurrency]string{
	BAT: "BAT",
	BTC: "BTC",
	ETH: "ETH",
	LTC: "LTC",
}

var altCurrencyId = map[string]AltCurrency{
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

func (a AltCurrency) IsValid() bool {
	_, exists := altCurrencyName[a]
	if !exists || a == INVALID {
		return false
	}
	return true
}

func (a AltCurrency) Scale() decimal.Decimal {
	return decimal.New(1, altCurrencyDecimals[a])
}

func (a AltCurrency) ToProbi(v decimal.Decimal) decimal.Decimal {
	return v.Mul(a.Scale())
}

func (a AltCurrency) FromProbi(v decimal.Decimal) decimal.Decimal {
	return v.DivRound(a.Scale(), altCurrencyDecimals[a])
}

func (a AltCurrency) String() string {
	return altCurrencyName[a]
}

func (a *AltCurrency) MarshalText() (text []byte, err error) {
	if *a == INVALID {
		return nil, errors.New("Not a valid AltCurrency")
	}
	text = []byte(a.String())
	return
}

func (a *AltCurrency) UnmarshalText(text []byte) (err error) {
	s := string(text)
	var exists bool
	*a, exists = altCurrencyId[s]
	if !exists {
		return errors.New("Not a valid AltCurrency")
	}
	return nil
}

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

// Copied from https://github.com/ethereum/go-ethereum/, licensed under the GNU General Public License v3.0
// Keccak256 calculates and returns the Keccak256 hash of the input data.
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

func ToChecksumETHAddress(str string) string {
	lower := strings.Replace(strings.ToLower(str), "0x", "", 1)
	lowerBytes := []byte(lower)
	hash := Keccak256([]byte(lower))
	hashHex := make([]byte, hex.EncodedLen(len(hash)))
	hex.Encode(hashHex, hash)

	for i, v := range lowerBytes {
		x, _ := strconv.ParseUint(string([]byte{hashHex[i]}), 16, 8)
		if x >= 8 {
			lowerBytes[i] = byte(unicode.ToUpper(rune(v)))
		}
	}
	return "0x" + string(lowerBytes)
}
