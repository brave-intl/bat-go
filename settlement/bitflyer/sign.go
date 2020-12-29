package bitflyersettlement

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	"github.com/brave-intl/bat-go/utils/vaultsigner"
)

// SignRequests signs formed requests
func SignRequests(
	clientID string,
	clientKey string,
	hmacSecret *vaultsigner.HmacSigner,
	privateRequests *[][]bitflyer.PayoutPayload,
) (*[]bitflyer.PrivateRequestSequence, error) {
	privateRequestSequences := make([]bitflyer.PrivateRequestSequence, 0)
	// sign each request

	if len(clientID) == 0 {
		return nil, errors.New("a client id was missing during the bitflyer settlement signing process")
	}

	for _, privateRequestRequirements := range *privateRequests {
		base := bitflyer.NewBulkPayoutPayload(
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
		requestSequence := bitflyer.PrivateRequestSequence{
			Signatures: signatures,
			Base:       base,
			APIKey:     clientKey,
		}
		privateRequestSequences = append(privateRequestSequences, requestSequence)
	}
	return &privateRequestSequences, nil
}
