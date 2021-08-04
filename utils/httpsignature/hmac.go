package httpsignature

import (
	"crypto"
	"crypto/hmac"
	"crypto/sha512"
	"crypto/subtle"
	"io"
)

type HMACKey string

func (key HMACKey) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	hhash := hmac.New(sha512.New, []byte(key))
	//  writing the message (HTTP signing string) to it
	hhash.Write(message)
	// Get the hash sum, do not base64 encode it since sig was decoded already
	return hhash.Sum(nil), nil
}

func (key HMACKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	hashSum, err := key.Sign(nil, message, nil)
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
