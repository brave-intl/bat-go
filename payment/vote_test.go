package payment

import (
	"context"
	"database/sql/driver"
	"encoding/base64"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/brave-intl/bat-go/datastore/grantserver"
	"github.com/brave-intl/bat-go/utils/clients/cbr"
)

type BytesContains []byte

func (bc BytesContains) Match(v driver.Value) bool {
	if b, ok := v.([]byte); ok {
		if strings.Contains(string(b), string(bc)) {
			return true
		}
		return false
	}
	return false
}

type StringContains string

func (sc StringContains) Match(v driver.Value) bool {
	if s, ok := v.(string); ok {
		if strings.Contains(s, string(sc)) {
			return true
		}
		return false
	}
	return false
}

// TestVoteAnonCard - given an issuer that is suffixed with "anon-card-vote" we should get a vote with funding source anon-card-vote
func TestVoteAnonCard(t *testing.T) {
	var (
		s                                 = new(Service)
		fakeGenerateCredentialRedemptions = func(ctx context.Context, cb []CredentialBinding) ([]cbr.CredentialRedemption, error) {
			return []cbr.CredentialRedemption{
				{
					Issuer: "brave.com.anon-card-vote",
				},
			}, nil
		}
		oldGenerateCredentialRedemptions = generateCredentialRedemptions
		db, mock, _                      = sqlmock.New()
	)
	// avro codecs
	s.InitCodecs()
	s.datastore = Datastore(&Postgres{grantserver.Postgres{sqlx.NewDb(db, "postgres")}})

	defer db.Close()
	voteText := base64.StdEncoding.EncodeToString([]byte(`{"channel":"brave.com", "type":"auto-contribute"}`))

	// make sure vote_drain was updated
	mock.ExpectExec("insert into vote_drain").
		WithArgs(StringContains(`issuer":"brave.com.anon-card-vote`), voteText, BytesContains(`anon-card-vote`)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	generateCredentialRedemptions = fakeGenerateCredentialRedemptions
	defer func() {
		generateCredentialRedemptions = oldGenerateCredentialRedemptions
	}()

	err := s.Vote(
		context.Background(), []CredentialBinding{}, voteText)
	if err != nil {
		t.Error("encountered an error in Vote call: ", err.Error())
	}
}
