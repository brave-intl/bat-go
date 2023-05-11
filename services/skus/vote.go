package skus

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/libs/clients/cbr"
	appctx "github.com/brave-intl/bat-go/libs/context"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
	"github.com/brave-intl/bat-go/libs/inputs"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/jmoiron/sqlx"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	kafka "github.com/segmentio/kafka-go"
	"github.com/shopspring/decimal"
)

const (
	// UserWalletVoteSKU - special vote sku to denote user wallet funding
	UserWalletVoteSKU string = "user-wallet-vote"
	// AnonCardVoteSKU - special vote sku to denote anon-card funding
	AnonCardVoteSKU = "anon-card-vote"
)

var (
	// ErrInvalidSKUToken - the sku was invalid
	ErrInvalidSKUToken = errors.New("failed to validate sku token")
	// ErrInvalidSKUTokenSKU - the sku was invalid
	ErrInvalidSKUTokenSKU = fmt.Errorf("invalid sku in sku token: %w", ErrInvalidSKUToken)
	// ErrInvalidSKUTokenBadMerchant - the merchant in the sku is invalid
	ErrInvalidSKUTokenBadMerchant = fmt.Errorf("invalid merchant id in sku token: %w", ErrInvalidSKUToken)
)

// Vote encapsulates information from the browser about attention
type Vote struct {
	Type          string `json:"type" valid:"in(auto-contribute|oneoff-tip|recurring-tip)"`
	Channel       string `json:"channel" valid:"-"`
	VoteTally     int64  `json:"-" valid:"-"`
	FundingSource string `json:"-" valid:"-"`
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

// VoteEvent encapsulates user and server provided information about a request to contribute kafka event
type VoteEvent struct {
	Type          string          `json:"type"`
	Channel       string          `json:"channel"`
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
			FundingSource: v.FundingSource,
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
		return errorutils.Wrap(err, "error decoding vote")
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

func rollbackTx(ds Datastore, tx *sqlx.Tx, wrap string, err error) error {
	if pg, ok := ds.(*Postgres); ok {
		if tx != nil {
			// will handle logging to sentry if there is an error
			pg.RollbackTx(tx)
		}
	}
	return errorutils.Wrap(err, wrap)
}

// RunNextVoteDrainJob - Attempt to drain the vote queue
func (s *Service) RunNextVoteDrainJob(ctx context.Context) (bool, error) {
	logger := logging.Logger(ctx, "skus.RunNextVoteDrainJob")

	select {
	case <-ctx.Done():
		// cancellation happened, kill this worker
		logger.Error().Msg("cancellation envoked in drain vote queue!\n")
		return false, nil
	default:

		// make sure we are not paused.
		if s.IsPaused() {
			logger.Error().Msg("drain worker is paused!\n")
			return false, nil
		}
		// pull vote from db queue
		tx, records, err := s.Datastore.GetUncommittedVotesForUpdate(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("failed to get uncommitted votes from drain queue")
			return true, rollbackTx(s.Datastore, tx, "failed to get uncommitted votes from drain queue", err)
		}
		for _, record := range records {
			if record == nil {
				continue
			}
			var requestCredentials = []cbr.CredentialRedemption{}
			err := json.Unmarshal([]byte(record.RequestCredentials), &requestCredentials)
			if err != nil {
				logger.Error().Err(err).Msg("failed to decode credentials")
				if err := s.Datastore.MarkVoteErrored(ctx, *record, tx); err != nil {
					logger.Error().Err(err).Msg("failed to mark vote as errored")
					return true, rollbackTx(s.Datastore, tx, "failed to mark vote as errored for creds redemption", err)
				}
				// okay if it is errored, we will update the errored column
			}
			// redeem the credentials
			err = s.cbClient.RedeemCredentials(ctx, requestCredentials, record.VoteText)
			if err != nil {
				logger.Error().Err(err).Msg("failed to redeem credentials")
				if err := s.Datastore.MarkVoteErrored(ctx, *record, tx); err != nil {
					return true, rollbackTx(s.Datastore, tx, "failed to mark vote as errored for creds redemption", err)
				}
				// okay if errored, update errored column
			}
			// write the message to kafka if successful
			if err = s.kafkaWriter.WriteMessages(ctx,
				kafka.Message{
					Topic: voteTopic,
					Value: record.VoteEventBinary,
				},
			); err != nil {
				if strings.Contains(err.Error(), "expired") {
					// pause the worker for 30 minutes, expired cert
					s.PauseWorker(time.Now().Add(30 * time.Minute))
				}
				logger.Error().Err(err).Msg("failed to write message to kafka")
				return true, rollbackTx(s.Datastore, tx, "failed to write vote to kafka", err)
			}
			// update the particular record to not be picked again
			if err = s.Datastore.CommitVote(ctx, *record, tx); err != nil {
				logger.Error().Err(err).Msg("failed to commit the vote")
				return true, rollbackTx(s.Datastore, tx, "failed to commit vote to drain vote queue", err)
			}
		}
		// finalize the record
		if err := tx.Commit(); err != nil {
			logger.Error().Err(err).Msg("failed to commit the transaction")
			return true, fmt.Errorf("failed to commit transaction in drain vote queue: %w", err)
		}
		return true, nil
	}
}

// Vote based on the browser's attention
func (s *Service) Vote(
	ctx context.Context, credentials []CredentialBinding, voteText string) error {

	logger := logging.Logger(ctx, "skus.Vote")

	var vote Vote
	// decode and validate the inputs
	if err := inputs.DecodeAndValidate(ctx, &vote, []byte(voteText)); err != nil {
		return fmt.Errorf("error performing input decode/validate: %w", err)
	}

	// generate all the cb credential redemptions
	requestCredentials, err := generateCredentialRedemptions(
		context.WithValue(ctx, appctx.DatastoreCTXKey, s.Datastore), credentials)
	if err != nil {
		return fmt.Errorf("error generating credential redemptions: %w", err)
	}

	var credsByIssuer = map[string][]cbr.CredentialRedemption{}

	if len(requestCredentials) > 0 {
		for _, rc := range requestCredentials {
			// validate the issuer / sku of all credentials for validation
			// we accept all the votes, or none of the votes.
			merchantID, sku, err := decodeIssuerID(rc.Issuer)
			if err != nil {
				return fmt.Errorf("failed to decode issuer name for sku: %w", err)
			}

			if merchantID != "brave.com" {
				// validate that the merchantID is brave.com
				// if not hard fail the request, and return an error stating the problem
				logger.Warn().Str("merchantID", merchantID).Msg("merchantID should be brave.com, vote invalid")
				return fmt.Errorf("merchant id != brave.com: %w", ErrInvalidSKUTokenBadMerchant)
			}

			if sku != UserWalletVoteSKU && sku != AnonCardVoteSKU {
				logger.Warn().Str("sku", sku).Msg("sku is invalid, should be user-wallet, or anonymous-card")
				return fmt.Errorf("%s is an invalid sku: %w", sku, ErrInvalidSKUTokenSKU)
			}

			// validation has completed.
			credsByIssuer[rc.Issuer] = append(credsByIssuer[rc.Issuer], rc)
		}
	}

	// for each issuer we will create a vote with a particular vote tally
	for k, v := range credsByIssuer {
		vote.VoteTally = int64(len(v))
		// k holds the issuer name string, which has encoded in the funding source
		// draw out the funding source and set it here.
		_, sku, err := decodeIssuerID(k)
		if err != nil {
			return fmt.Errorf("failed to decode issuer name for sku: %w", err)
		}
		switch sku {
		case UserWalletVoteSKU:
			vote.FundingSource = "user-wallet"
		case AnonCardVoteSKU:
			vote.FundingSource = "anonymous-card"
		default:
			// should not get here, doing validation above on each issuer name
			logger.Warn().Str("sku", sku).Msg("sku is invalid, should be user-wallet, or anonymous-card")
			return fmt.Errorf("%s is an invalid sku: %w", sku, ErrInvalidSKUTokenSKU)
		}

		// get a new VoteEvent to emit to kafka based on our input vote
		voteEvent, err := NewVoteEvent(vote)
		if err != nil {
			return fmt.Errorf("failed to convert vote to kafka vote event: %w", err)
		}

		// encode the event for processing
		voteEventBinary, err := voteEvent.CodecEncode(s.codecs["vote"])
		if err != nil {
			return fmt.Errorf("failed to encode avro codec: %w", err)
		}

		rcSerial, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to encode request credentials for vote drain: %w", err)
		}

		// insert serialized event into db
		if err = s.Datastore.InsertVote(
			ctx, VoteRecord{
				RequestCredentials: string(rcSerial),
				VoteText:           voteText,
				VoteEventBinary:    voteEventBinary,
			}); err != nil {
			return fmt.Errorf("datastore failure vote_drain: %w", err)
		}
	}
	// at this point, after the vote is added to the database queue, we will let
	// the service DrainVoteQueue handle the redemptions and kafka messages

	return nil
}
