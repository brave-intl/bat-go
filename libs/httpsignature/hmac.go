package httpsignature

import (
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
)

// HMACKey is a symmetric key that can be used for HMAC-SHA512 request signing and verification
type HMACKey string

// Sign the message using the hmac key
func (key HMACKey) SignMessage(message []byte) (signature []byte, err error) {
	hhash := hmac.New(sha512.New, []byte(key))
	//  writing the message (HTTP signing string) to it
	_, err = hhash.Write(message)
	if err != nil {
		return nil, err
	}
	// Get the hash sum, do not base64 encode it since sig was decoded already
	return hhash.Sum(nil), nil
}

// Verify the signature sig for message using the hmac key
func (key HMACKey) VerifySignature(message, sig []byte) error {
	hashSum, err := key.SignMessage(message)
	if err != nil {
		return err
	}
	// Return bool by checking whether or not the calculated hash is equal to
	// sig pulled out of the header. Check if returned int is equal to 1 to return a bool
	if subtle.ConstantTimeCompare(hashSum, sig) != 1 {
		return ErrBadSignature
	}
	return nil
}

func (key HMACKey) String() string {
	return string(key)
}
