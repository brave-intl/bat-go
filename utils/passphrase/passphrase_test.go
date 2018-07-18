package passphrase

import (
	"crypto"
	"crypto/rand"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ed25519"
)

var array16 []byte
var array32 []byte

func init() {
	array16 = make([]byte, 16)
	array32 = make([]byte, 32)
	for i := 0; i < 32; i++ {
		if i < 16 {
			array16[i] = 255
		}
		array32[i] = 255
	}
}

func TestDeriveSigningKeysFromSeed(t *testing.T) {
	hkdfSalt := []byte{72, 203, 156, 43, 64, 229, 225, 127, 214, 158, 50, 29, 130, 186, 182, 207, 6, 108, 47, 254, 245, 71, 198, 109, 44, 108, 32, 193, 221, 126, 119, 143, 112, 113, 87, 184, 239, 231, 230, 234, 28, 135, 54, 42, 9, 243, 39, 30, 179, 147, 194, 211, 212, 239, 225, 52, 192, 219, 145, 40, 95, 19, 142, 98}
	seed, err := hex.DecodeString("5bb5ceb168e4c8e26a1a16ed34d9fc7fe92c1481579338da362cb8d9f925d7cb")

	if err != nil {
		t.Error("Unexpected error decoding hex")
	}

	key, err := DeriveSigningKeysFromSeed(seed, hkdfSalt)
	if err != nil {
		t.Error("Unexpected error deriving keys")
	}

	if hex.EncodeToString(key) != "b5abda6940984c5153a2ba3653f047f98dfb19e39c3e02f07c8bbb0bd8e8872ef58ca446f0c33ee7e8e9874466da442b2e764afd77ad46034bdff9e01f9b87d4" {
		t.Error("Wrong private key!")
	}

	pk := key.Public().(ed25519.PublicKey)
	if hex.EncodeToString(pk) != "f58ca446f0c33ee7e8e9874466da442b2e764afd77ad46034bdff9e01f9b87d4" {
		t.Error("Wrong public key!")
	}

	message := []byte("€ 123 ッッッ　あ")
	sig, err := key.Sign(rand.Reader, message, crypto.Hash(0))
	if err != nil {
		t.Error("Unexpected error signing message")
	}

	// nacl combined signature mode is equivalent to the message with signature prepended
	if hex.EncodeToString(sig)+hex.EncodeToString(message) != "81d39235e6e1ed07e7d2cee6380aa07f415ab4b43cc4cef4043e61171ac5511253807105aba9b7b9c9cdbb0e84517178176d87f757801275a16477a51013e10de282ac2031323320e38383e38383e38383e38080e38182" {
		t.Error("Signature did not match expected value from brave/crypto")
	}

	if !ed25519.Verify(pk, message, sig) {
		t.Error("Signature verification failed")
	}

	sig[0] = 255
	if ed25519.Verify(pk, message, sig) {
		t.Error("Signature verification should have failed")
	}
}

func TestFromHex(t *testing.T) {
	phrase, err := FromHex("00000000000000000000000000000000")
	if err != nil {
		t.Error("Error during hex to bip39 phrase")
	}
	if strings.Join(phrase, " ") != "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about" {
		t.Error("bip39 phrase did not match")
	}
}

func TestFromBytes(t *testing.T) {
	phrase, err := FromBytes(array16)
	if err != nil {
		t.Error("Error during hex to bip39 phrase")
	}
	if strings.Join(phrase, " ") != "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong" {
		t.Error("bip39 phrase did not match")
	}

	phrase, err = FromBytes(array32)
	if err != nil {
		t.Error("Error during hex to bip39 phrase")
	}
	if strings.Join(phrase, " ") != "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote" {
		t.Error("bip39 phrase did not match")
	}
}

func TestToBytes32(t *testing.T) {
	result, err := ToBytes32("a a a a a a a a a a a a a a a a")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if !reflect.DeepEqual(result, make([]byte, 32)) {
		t.Error("Resulting bytes did not match expectation")
	}

	result, err = ToBytes32(" zyzzyva  zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if !reflect.DeepEqual(result, array32) {
		t.Error("Resulting bytes did not match expectation")
	}

	result, err = ToBytes32("zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if !reflect.DeepEqual(result, array32) {
		t.Error("Resulting bytes did not match expectation")
	}

	_, err = ToBytes32("zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong")
	if err == nil {
		t.Error("Expected error due to incorrect phrase length")
	}
}

func TestToHex32(t *testing.T) {
	result, err := ToHex32("horsepox tiglon monolithic impoundment classiest propagation deviant temporize precessed sunburning pricey spied plack batcher overpassed bioengineering")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if result != "65f9e2ea89dd6a8d2333ab0b3808e011a757da60a95cd201a2e40df098f111d4" {
		t.Error("Resulting hex did not match expectation")
	}

	result, err = ToHex32(" zyzzyva  zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva zyzzyva")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if result != "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" {
		t.Error("Resulting hex did not match expectation")
	}

	result, err = ToHex32("zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo vote")
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if result != "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" {
		t.Error("Resulting hex did not match expectation")
	}

	_, err = ToHex32("abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about")
	if err == nil {
		t.Error("Expected error due to incorrect phrase length")
	}
}

func TestOriginalSeedCanBeRecovered(t *testing.T) {
	hex := "65f9e2ea89dd6a8d2333ab0b3808e011a757da60a95cd201a2e40df098f111d4"
	phrase, err := FromHex(hex)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	oHex, err := ToHex32(strings.Join(phrase, " "))
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if oHex != hex {
		t.Error("Resulting hex did not match original")
	}

	b := make([]byte, 32)
	_, err = rand.Read(b)
	if err != nil {
		t.Error("Unexpected error on random read")
	}

	phrase, err = FromBytes(b)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	ob, err := ToBytes32(strings.Join(phrase, " "))
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if !reflect.DeepEqual(ob, b) {
		t.Error("Resulting bytes did not match original")
	}
}

func TestOriginalPhraseCanBeRecovered(t *testing.T) {
	phrase := "magic vacuum wide review love peace century egg burden clutch heart cycle annual mixed pink awesome extra client cry brisk priority maple mountain jelly"

	hex, err := ToHex32(phrase)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	oPhrase, err := FromHex(hex)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if phrase != strings.Join(oPhrase, " ") {
		t.Error("Resulting phrase did not match original")
	}

	b, err := ToBytes32(phrase)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	oPhrase, err = FromBytes(b)
	if err != nil {
		t.Error("Unexpected error on valid phrase")
	}
	if phrase != strings.Join(oPhrase, " ") {
		t.Error("Resulting phrase did not match original")
	}
}
