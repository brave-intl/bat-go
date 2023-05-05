package skus

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/brave-intl/bat-go/libs/clients/cbr"
	"github.com/brave-intl/bat-go/libs/datastore"
	kafkautils "github.com/brave-intl/bat-go/libs/kafka"
	"github.com/brave-intl/bat-go/services/skus/db/repository"
)

type BytesContains []byte

func (bc BytesContains) Match(v driver.Value) bool {
	if b, ok := v.([]byte); ok {
		return strings.Contains(string(b), string(bc))
	}
	return false
}

type StringContains string

func (sc StringContains) Match(v driver.Value) bool {
	if s, ok := v.(string); ok {
		return strings.Contains(s, string(sc))
	}
	return false
}

// TestVoteAnonCard - given an issuer that is suffixed with "anon-card-vote" we should get a vote with funding source anon-card-vote
func TestVoteAnonCard(t *testing.T) {
	var (
		issuerName                        = "brave.com?sku=anon-card-vote"
		s                                 = new(Service)
		fakeGenerateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
			return []cbr.CredentialRedemption{
				{
					Issuer: issuerName,
				},
			}, nil
		}
		oldGenerateCredentialRedemptions = generateCredentialRedemptions
		db, mock, _                      = sqlmock.New()
		err                              error
	)
	// avro codecs
	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote": voteSchema,
	})
	if err != nil {
		t.Error("failed to initialize avro codecs for test: ", err)
	}
	s.Datastore = Datastore(
		&Postgres{
			Postgres: datastore.Postgres{
				DB: sqlx.NewDb(db, "postgres"),
			},
			orderRepo: repository.NewOrder(),
		},
	)

	defer func() {
		if err := db.Close(); err != nil {
			t.Log("failed to close mock database", err)
		}
	}()
	voteText := base64.StdEncoding.EncodeToString([]byte(`{"channel":"brave.com", "type":"auto-contribute"}`))

	// make sure vote_drain was updated
	mock.ExpectExec("insert into vote_drain").
		WithArgs(StringContains(`issuer":"`+issuerName), voteText, BytesContains(`anonymous-card`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	generateCredentialRedemptions = fakeGenerateCredentialRedemptions
	defer func() {
		generateCredentialRedemptions = oldGenerateCredentialRedemptions
	}()

	err = s.Vote(
		context.Background(), []CredentialBinding{}, voteText)
	if err != nil {
		t.Error("encountered an error in Vote call: ", err.Error())
	}
}

// TestVoteUnknownSKU - given an issuer that is suffixed with "anon-card-vote" we should get a vote with funding source anon-card-vote
func TestVoteUnknownSKU(t *testing.T) {
	var (
		issuerName                        = "brave.com?sku=bad-sku"
		s                                 = new(Service)
		fakeGenerateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
			return []cbr.CredentialRedemption{
				{
					Issuer: issuerName,
				},
			}, nil
		}
		oldGenerateCredentialRedemptions = generateCredentialRedemptions
		err                              error
	)
	// avro codecs
	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote": voteSchema,
	})
	if err != nil {
		t.Error("failed to initialize avro codecs for test: ", err)
	}
	voteText := base64.StdEncoding.EncodeToString([]byte(`{"channel":"brave.com", "type":"auto-contribute"}`))

	generateCredentialRedemptions = fakeGenerateCredentialRedemptions
	defer func() {
		generateCredentialRedemptions = oldGenerateCredentialRedemptions
	}()

	err = s.Vote(
		context.Background(), []CredentialBinding{}, voteText)
	if err == nil {
		t.Error("should have encountered an error in Vote call: ")
	}
	if !errors.Is(err, ErrInvalidSKUTokenSKU) {
		t.Error("invalid error, should have gotten invalidskutokensku: ", err)
	}
}

// TestVoteUnknownBadMerch - given an issuer that is suffixed with "anon-card-vote" we should get a vote with funding source anon-card-vote
func TestVoteUnknownBadMerch(t *testing.T) {
	var (
		issuerName                        = "notbrave.com?sku=anon-card-vote"
		s                                 = new(Service)
		fakeGenerateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
			return []cbr.CredentialRedemption{
				{
					Issuer: issuerName,
				},
			}, nil
		}
		oldGenerateCredentialRedemptions = generateCredentialRedemptions
		err                              error
	)
	// avro codecs
	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote": voteSchema,
	})
	if err != nil {
		t.Error("failed to initialize avro codecs for test: ", err)
	}
	voteText := base64.StdEncoding.EncodeToString([]byte(`{"channel":"brave.com", "type":"auto-contribute"}`))

	generateCredentialRedemptions = fakeGenerateCredentialRedemptions
	defer func() {
		generateCredentialRedemptions = oldGenerateCredentialRedemptions
	}()

	err = s.Vote(
		context.Background(), []CredentialBinding{}, voteText)
	if err == nil {
		t.Error("should have encountered an error in Vote call: ")
	}
	if !errors.Is(err, ErrInvalidSKUTokenBadMerchant) {
		t.Error("invalid error, should have gotten invalidskutokenbadmerchant: ", err)
	}
}

// TestVoteGoodAndBadSKU - one good credential, and one bad credential should result in no votes
func TestVoteGoodAndBadSKU(t *testing.T) {
	var (
		issuerName1                       = "brave.com?sku=bad-sku"
		issuerName2                       = "brave.com?sku=bad-sku"
		s                                 = new(Service)
		fakeGenerateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
			return []cbr.CredentialRedemption{
				{
					Issuer: issuerName1,
				},
				{
					Issuer: issuerName2,
				},
			}, nil
		}
		oldGenerateCredentialRedemptions = generateCredentialRedemptions
		err                              error
	)
	// avro codecs
	s.codecs, err = kafkautils.GenerateCodecs(map[string]string{
		"vote": voteSchema,
	})
	if err != nil {
		t.Error("failed to initialize avro codecs for test: ", err)
	}
	voteText := base64.StdEncoding.EncodeToString([]byte(`{"channel":"brave.com", "type":"auto-contribute"}`))

	generateCredentialRedemptions = fakeGenerateCredentialRedemptions
	defer func() {
		generateCredentialRedemptions = oldGenerateCredentialRedemptions
	}()

	err = s.Vote(
		context.Background(), []CredentialBinding{}, voteText)
	if err == nil {
		t.Error("should have encountered an error in Vote call: ")
	}

	if !errors.Is(err, ErrInvalidSKUTokenSKU) {
		t.Error("invalid error, should have gotten invalidskutokensku: ", err)
	}
}
