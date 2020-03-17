package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
)

// CredentialBinding includes info needed to redeem a single credential
type CredentialBinding struct {
	PublicKey     string `json:"publicKey" valid:"base64"`
	TokenPreimage string `json:"t" valid:"base64"`
	Signature     string `json:"signature" valid:"base64"`
}

// Vote encapsulates information from the browser about attention
type Vote struct {
	Type    string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel string `json:"channel" valid:"-"`
}

// Base64Decode unmarshalls the vote from a string.
func (s *Vote) Base64Decode(text string) error {
	var bytes []byte
	bytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, s)
	return err
}

// Vote based on the browser's attention
func (service *Service) Vote(ctx context.Context, credentials []CredentialBinding, voteText string) error {
	var vote Vote
	err := vote.Base64Decode(voteText)
	if err != nil {
		return errorutils.Wrap(err, "Error decoding vote")
	}

	_, err = govalidator.ValidateStruct(vote)
	if err != nil {
		return err
	}

	requestCredentials := make([]cbr.CredentialRedemption, len(credentials))
	issuers := make(map[string]*Issuer)

	for i := 0; i < len(credentials); i++ {
		var ok bool
		var issuer *Issuer

		publicKey := credentials[i].PublicKey

		if issuer, ok = issuers[publicKey]; !ok {
			issuer, err = service.datastore.GetIssuerByPublicKey(publicKey)
			if err != nil {
				return errorutils.Wrap(err, "Error finding issuer")
			}
		}

		requestCredentials[i].Issuer = issuer.Name()
		requestCredentials[i].TokenPreimage = credentials[i].TokenPreimage
		requestCredentials[i].Signature = credentials[i].Signature
	}

	// FIXME insert serialized event into db

	go func() {
		err = service.cbClient.RedeemCredentials(ctx, requestCredentials, voteText)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}

		// FIXME emit kafka event

		fmt.Println("Valid vote recieved:", voteText)
	}()

	return nil
}
