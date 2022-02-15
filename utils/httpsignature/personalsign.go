package httpsignature

import (
	"crypto"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	ethc "github.com/ethereum/go-ethereum/crypto"
)

// EthAddress - the ethereum address hex repr
type EthAddress common.Address

// Verify the signature sig for message based on eth address
func (key EthAddress) Verify(message, sig []byte, opts crypto.SignerOpts) (bool, error) {
	m := fmt.Sprintf("Claim Brave Swap Rewards BAT\n%s", []byte(message))
	// recover the public key that created the signature
	hash := accounts.TextHash([]byte(m))

	pubKey, err := ethc.SigToPub(hash, sig)
	if err != nil {
		return false, err
	}

	if EthAddress(ethc.PubkeyToAddress(*pubKey)) == key {
		// address matches, perform verification
		return ethc.VerifySignature(ethc.FromECDSAPub(pubKey), hash, sig), nil
	}

	return false, nil
}
