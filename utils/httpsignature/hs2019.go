package httpsignature

import (
	"fmt"
	"crypto"
	"crypto/sha512"
	"crypto/subtle"
	"crypto/hmac"
	"encoding/base64"
)

type HMACKey string

func (key HMACKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	hhash := hmac.New(sha512.New, []byte(key))
	hhash.Write(message)
	calcHash := base64.StdEncoding.EncodeToString(hhash.Sum(nil))
	fmt.Println("Calc hash:", calcHash)
	calcHashByte := []byte(calcHash)
	
	
	fmt.Println("message:", calcHashByte)
	fmt.Println("signature:", sig)
	valid := subtle.ConstantTimeCompare(calcHashByte, sig)
	fmt.Println("HMAC valid:", valid)
	
	valid2 := hmac.Equal(calcHashByte, sig)
	fmt.Println("Vaild 2:", valid2)
	return valid != 0, nil
}

func (key HMACKey) String() string {
	return string(key)
}