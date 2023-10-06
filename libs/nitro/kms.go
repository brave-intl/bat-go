package nitro

import (
	"github.com/brave-intl/bat-go/libs/nitro/ber"
	"github.com/brave-intl/bat-go/libs/nitro/pkcs7"

	"crypto/rsa"
)

// Decrypt the encrypted response from KMS
func Decrypt(key *rsa.PrivateKey, b []byte) ([]byte, error) {
	der, err := ber.ToDER(b)
	if err != nil {
		return nil, err
	}

	pkcs, err := pkcs7.Parse(der)
	if err != nil {
		return nil, err
	}

	return pkcs.Decrypt(key)
}
