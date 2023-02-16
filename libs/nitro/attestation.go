package nitro

import (
	"errors"
	"log"

	"github.com/hf/nsm"
	"github.com/hf/nsm/request"
)

// Attest takes as input a nonce, user-provided data and a public key, and then
// asks the Nitro hypervisor to return a signed attestation document that
// contains all three values.
func Attest(nonce, userData, publicKey []byte) ([]byte, error) {
	s, err := nsm.OpenDefaultSession()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = s.Close(); err != nil {
			log.Printf("Attestation: Failed to close default NSM session: %s", err)
		}
	}()

	res, err := s.Send(&request.Attestation{
		Nonce:     nonce,
		UserData:  userData,
		PublicKey: publicKey,
	})
	if err != nil {
		return nil, err
	}

	if res.Attestation == nil || res.Attestation.Document == nil {
		return nil, errors.New("NSM device did not return an attestation")
	}

	return res.Attestation.Document, nil
}
