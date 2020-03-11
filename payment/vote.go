package payment

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/inputs"
	"github.com/jmoiron/sqlx"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

// Vote encapsulates information from the browser about attention
type Vote struct {
	Type      string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel   string `json:"channel" valid:"-"`
	VoteTally int64  `json:"voteTally"`
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
func (v *Vote) Base64Decode(text string) error {
	var bytes []byte
	bytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return err
	}

	err = json.Unmarshal(bytes, v)
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
	VoteTally     int64           `json:"voteTally"`
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
			VoteTally:     v.VoteTally,
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
	return codec.BinaryFromNative(nil, map[string]interface{}{
		"type":          ve.Type,
		"channel":       ve.Channel,
		"id":            ve.ID.String(),
		"createdAt":     ve.CreatedAt.Format(time.RFC3339),
		"baseVoteValue": ve.BaseVoteValue.String(),
		"voteTally":     ve.VoteTally,
		"fundingSource": ve.FundingSource,
	})
}

// CodecDecode - Decode using avro vote codec
func (ve *VoteEvent) CodecDecode(codec *goavro.Codec, binary []byte) error {
	native, _, err := codec.NativeFromBinary(binary)
	if err != nil {
		return fmt.Errorf("unable to decode avro payload: %w", err)
	}

	// gross
	v, err := json.Marshal(native)
	if err != nil {
		return fmt.Errorf("unable to marshal avro payload: %w", err)
	}

	err = json.Unmarshal(v, ve)
	if err != nil {
		return fmt.Errorf("unable to decode decoded avro payload to VoteEvent: %w", err)
	}

	return nil
}

func rollbackAndLogTx(tx *sqlx.Tx, wrap string, err error) {
	log.Printf(wrap+": %s", err)
	if tx != nil {
		if err := tx.Rollback(); err != nil {
			log.Printf("failed to rollback transaction in drain vote queue: %s", err)
		}
	}
}

// DrainVoteQueue - event loop that waits a certain amount of time, and then
// grabs from the vote queue and applies redemptions.
func (service *Service) DrainVoteQueue(ctx context.Context, cadence time.Duration) {
	go func(ctx context.Context) {
	OUTER:
		for {
			select {
			case <-ctx.Done():
				// cancellation happened, kill this worker
				log.Printf("cancellation envoked in drain vote queue!\n")
				return
			case <-time.After(cadence):
				// pull vote from db queue
				tx, records, err := service.datastore.GetUncommittedVotesForUpdate(ctx)
				if err != nil {
					rollbackAndLogTx(tx, "failed to get uncommitted votes from drain queue", err)
					continue OUTER
				}
				for _, record := range records {
					if record == nil {
						continue
					}
					var requestCredentials = []cbr.CredentialRedemption{}
					err := json.Unmarshal([]byte(record.RequestCredentials), &requestCredentials)
					if err != nil {
						// mark errored?
						log.Printf("failed to decode credentials: %s", err)
						if err := service.datastore.MarkVoteErrored(ctx, *record, tx); err != nil {
							rollbackAndLogTx(tx, "failed to marke vote as errored for creds redemption", err)
							continue OUTER
						}
						// okay if it is errored, we will update the errored column
					}
					// redeem the credentials
					err = service.cbClient.RedeemCredentials(ctx, requestCredentials, record.VoteText)
					if err != nil {
						log.Printf("failed to redeem credentials: %s", err)
						// mark errored?
						if err := service.datastore.MarkVoteErrored(ctx, *record, tx); err != nil {
							rollbackAndLogTx(tx, "failed to marke vote as errored for creds redemption", err)
							continue OUTER
						}
						// okay if errored, update errored column
					}
					// write the message to kafka if successful
					if err = service.kafkaWriter.WriteMessages(ctx,
						kafka.Message{
							Value: record.VoteEventBinary,
						},
					); err != nil {
						rollbackAndLogTx(tx, "failed to write vote to kafka", err)
						continue OUTER
					}
					// update the particular record to not be picked again
					if err = service.datastore.CommitVote(ctx, *record, tx); err != nil {
						log.Printf("failed to commit vote in drain vote queue: %s", err)
						rollbackAndLogTx(tx, "failed to commit vote to drain vote queue", err)
						continue OUTER
					}
				}
				// finalize the record
				if err := tx.Commit(); err != nil {
					log.Printf("failed to commit transaction in drain vote queue: %s", err)
					continue OUTER
				}
			}
		}
	}(ctx)
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
		context.WithValue(ctx, appctx.DatastoreCTXKey, service.datastore), credentials)
	if err != nil {
		return fmt.Errorf("error generating credential redemptions: %w", err)
	}

	// get a new VoteEvent to emit to kafka based on our input vote
	voteEvent, err := NewVoteEvent(vote)
	if err != nil {
		return fmt.Errorf("failed to convert vote to kafka vote event: %w", err)
	}

	// encode the event for processing
	voteEventBinary, err := voteEvent.CodecEncode(service.codecs["vote"])
	if err != nil {
		return fmt.Errorf("failed to encode avro codec: %w", err)
	}

	rcSerial, err := json.Marshal(requestCredentials)
	if err != nil {
		return fmt.Errorf("failed to encode request credentials for vote drain: %w", err)
	}

	// insert serialized event into db
	if err = service.datastore.InsertVote(
		ctx, VoteRecord{
			RequestCredentials: string(rcSerial),
			VoteText:           voteText,
			VoteEventBinary:    voteEventBinary,
		}); err != nil {
		return fmt.Errorf("datastore failure vote_drain: %w", err)
	}

	// at this point, after the vote is added to the database queue, we will let
	// the service DrainVoteQueue handle the redemptions and kafka messages

	return nil
}
