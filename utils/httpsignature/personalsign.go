package httpsignature

import (
	"crypto"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	ethc "github.com/ethereum/go-ethereum/crypto"
)

// EthAddress - the ethereum address hex repr
type EthAddress string

// Verify the signature sig for message based on eth address
func (key EthAddress) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {

	// convert key to eth address
	address := common.HexToAddress(string(key))

	// recover the public key that created the signature
	hash := accounts.TextHash(message)

	pubKey, err := ethc.SigToPub(hash, sig)
	if err != nil {
		return false, err
	}

	if ethc.PubkeyToAddress(*pubKey) == address {
		// address matches, perform verification
		return ethc.VerifySignature(ethc.FromECDSAPub(pubKey), hash, sig), nil
	}

	return false, nil
}

// String - implement stringer
func (key EthAddress) String() string {
	return string(key)
}
