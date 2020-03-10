package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

// Vote encapsulates information from the browser about attention
type Vote struct {
	Type    string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel string `json:"channel" valid:"-"`
}

// Validate - implement inputs.Validatable interface for input
func (v *Vote) Validate(ctx context.Context) error {
	_, err := govalidator.ValidateStruct(v)
	if err != nil {
		return fmt.Errorf("failed vote structure validation: %w", err)
	}
	return nil
}

// Decode - implement inputs.Decodable interface for input
func (v *Vote) Decode(ctx context.Context, input []byte) error {
	err := v.Base64Decode(string(input))
	if err != nil {
		return fmt.Errorf("error decoding vote: %w", err)
	}
	return nil
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

/*
	{
		"type": "auto-contribute",
		"channel": "coinmarketcap.com",
		"voteTally": 20,
		"fundingSource": "uphold"
	}
*/

// VoteEvent encapsulates user and server provided information about a request to contribute
type VoteEvent struct {
	Type          string          `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel       string          `json:"channel" valid:"-"`
	ID            uuid.UUID       `json:"id"`
	CreatedAt     time.Time       `json:"createdAt"`
	BaseVoteValue decimal.Decimal `json:"baseVoteValue"`
	voteTally     int64           `json:"voteTally"`
	FundingSource string          `json:"fundingSource"`
}

// NewVoteEvent - Create a new VoteEvent given a Vote
func NewVoteEvent(v Vote) (*VoteEvent, error) {
	var (
		ve = &VoteEvent{
			ID:            uuid.NewV4(),
			Type:          v.Type,
			Channel:       v.Channel,
			CreatedAt:     time.Now().UTC(),
			FundingSource: "uphold",
		}
		err error
	)
	// default base vote value
	if ve.BaseVoteValue, err = decimal.NewFromString("0.25"); err != nil {
		return nil, fmt.Errorf("failed to default BaseVoteValue: %w", err)
	}

	return ve, nil
}

// CodecEncode - encode using avro vote codec
func (ve *VoteEvent) CodecEncode(codec *goavro.Codec) ([]byte, error) {
	return codec.BinaryFromNative(nil, ve)
}

// Vote based on the browser's attention
func (service *Service) Vote(
	ctx context.Context, credentials []CredentialBinding, voteText string) error {

	var vote Vote
	// decode and validate the inputs
	if err := inputs.DecodeAndValidate(ctx, &vote, []byte(voteText)); err != nil {
		return fmt.Errorf("error performing input decode/validate: %w", err)
	}

	// generate all the cb credential redemptions
	requestCredentials, err := generateCredentialRedemptions(
		context.WithValue(ctx, datastoreCTXKey, service.datastore), credentials)
	if err != nil {
		return fmt.Errorf("error generating credential redemptions: %w", err)
	}

	// get a new VoteEvent to emit to kafka based on our input vote
	voteEvent, err := NewVoteEvent(vote)

	// TODO insert serialized event into db

	go func() {
		err = service.cbClient.RedeemCredentials(ctx, requestCredentials, voteText)
		if err != nil {
			return
		}
		// TODO: update db to say we redeemed creds right...?

		v, err := voteEvent.CodecEncode(service.codecs["vote"])
		if err != nil {
			log.Printf("failed to encode avro codec: %s", err)
		}

		// write the message to kafka
		if err = service.kafkaWriter.WriteMessages(ctx,
			kafka.Message{
				Value: v,
			},
		); err != nil {
			log.Printf("failed to write to kafka: %s", err)
		}
		log.Printf("vote submitted for processing: %v", voteEvent)
	}()

	return nil
}
