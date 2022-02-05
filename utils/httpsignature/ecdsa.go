package httpsignature

import (
	"crypto"
	"crypto/subtle"
	"io"

	ethc "github.com/ethereum/go-ethereum/crypto"
)

// ECDSAKey is a symmetric key that can be used for ECDSA request signing and verification
type ECDSAKey string

// Sign the message using the ecdsa key
func (key ECDSAKey) Sign(rand io.Reader, message []byte, opts crypto.SignerOpts) (signature []byte, err error) {
	privateKey, err := ethc.HexToECDSA(string(key))
	if err != nil {
		return nil, err
	}
	hash := ethc.Keccak256Hash(message)
	signature, sigerr := ethc.Sign(hash.Bytes(), privateKey)
	if sigerr != nil {
		return nil, sigerr
	}

	return signature, nil
}

// Verify the signature sig for message using the ecdsa key
func (key ECDSAKey) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	hashSum, err := ethc.Ecrecover(message, sig)
	if err != nil {
		return false, err
	}
	// Return bool by checking whether or not the calculated hash is equal to
	// sig pulled out of the header. Check if returned int is equal to 1 to return a bool
	return subtle.ConstantTimeCompare(hashSum, sig) == 1, nil
}

func (key ECDSAKey) String() string {
	return string(key)
}
