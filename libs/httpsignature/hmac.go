package httpsignature

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"io"
)

// HMACKey is a symmetric key that can be used for HMAC-SHA512 request signing and verification
type HMACKey string

func hmacSign(key HMACKey, message []byte) ([]byte, error) {
	hhash := hmac.New(sha512.New, []byte(key))
	//  writing the message (HTTP signing string) to it
	_, err := hhash.Write(message)
	if err != nil {
		return nil, err
	}
	// Get the hash sum, do not base64 encode it since sig was decoded already
	return hhash.Sum(nil), nil
}

// Sign the message using the hmac key
func (key HMACKey) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) ([]byte, error) {
	return hmacSign(key, message)
}

// Verify the signature sig for message using the hmac key
func (key HMACKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	hashSum, err := hmacSign(key, message)
	if err != nil {
		return false, err
	}
	// Return bool by checking whether or not the calculated hash is equal to
	// sig pulled out of the header. Check if returned int is equal to 1 to return a bool
	return subtle.ConstantTimeCompare(hashSum, sig) == 1, nil
}

func (key HMACKey) String() string {
	return string(key)
}
