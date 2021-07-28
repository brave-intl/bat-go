package httpsignature

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
)

type HMACKey string

func (key HMACKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	// Recalculate the hash by setting up the hash
	hhash := hmac.New(sha512.New, []byte(key))
	// then writing the message (HTTP headers) to it
	hhash.Write(message)
	// Get the hash sum, do not base64 encode it since sig was decoded already
	hashSum := hhash.Sum(nil)
	
	// Return bool by checking whether or not the calculated hash is equal to
	// sig pulled out of the header. Check if returned int is equal to 1 to return a bool
	return subtle.ConstantTimeCompare(hashSum, sig) == 1, nil
}

func (key HMACKey) String() string {
	return string(key)
}