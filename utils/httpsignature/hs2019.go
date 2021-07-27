package httpsignature

import (
	"fmt"
	"crypto"
	"crypto/subtle"
	"crypto/hmac"
)

type HMACKey string

func (key HMACKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	fmt.Println("message:", message)
	fmt.Println("signature:", sig)
	valid := subtle.ConstantTimeCompare(message, sig)
	fmt.Println("HMAC valid:", valid)
	
	valid2 := hmac.Equal(message, sig)
	fmt.Println("Vaild 2:", valid2)
	return valid != 0, nil
}

func (key HMACKey) String() string {
	return key
}