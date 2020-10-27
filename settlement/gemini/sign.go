package geminisettlement

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"

	"github.com/brave-intl/bat-go/utils/clients/gemini"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
)

// SignRequests signs formed requests
func SignRequests(
	clientID string,
	clientKey string,
	hmacSecret *vaultsigner.HmacSigner,
	privateRequests *[][]gemini.PayoutPayload,
) (*[]gemini.PrivateRequestSequence, error) {
	privateRequestSequences := make([]gemini.PrivateRequestSequence, 0)
	// sign each request
	for _, privateRequestRequirements := range *privateRequests {
		base := gemini.NewBulkPayoutPayload(
			nil,
			clientID,
			&privateRequestRequirements,
		)
		signatures := []string{}
		// store the original nonce
		originalNonce := base.Nonce
		for i := 0; i < 10; i++ {
			// increment the nonce to correspond to each signature
			base.Nonce = originalNonce + int64(i)
			marshalled, err := json.Marshal(base)
			if err != nil {
				return nil, err
			}
			serializedPayload := base64.StdEncoding.EncodeToString(marshalled)
			sig, err := hmacSecret.HMACSha384(
				[]byte(serializedPayload),
			)
			if err != nil {
				return nil, err
			}
			signatures = append(signatures, hex.EncodeToString(sig))
		}
		base.Nonce = originalNonce
		requestSequence := gemini.PrivateRequestSequence{
			Signatures: signatures,
			Base:       base,
			APIKey:     clientKey,
		}
		privateRequestSequences = append(privateRequestSequences, requestSequence)
	}
	return &privateRequestSequences, nil
}
