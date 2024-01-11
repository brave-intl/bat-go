package model

import (
	"encoding/base64"
	"time"

	uuid "github.com/satori/go.uuid"
)

const (
	ErrWalletNotWhitelisted Error = "model: wallet not whitelisted"
	ErrNotFound             Error = "model: not found"
	ErrChallengeNotFound    Error = "model: challenge not found"
	ErrChallengeExpired     Error = "model: challenge expired"
	ErrNoRowsDeleted        Error = "model: no rows deleted"
	ErrNotInserted          Error = "model: not inserted"
	ErrNoWalletCustodian    Error = "model: no linked wallet custodian"
	ErrInternalServer       Error = "model: internal server error"
	ErrWalletNotFound       Error = "model: wallet not found"
)

type AllowListEntry struct {
	PaymentID uuid.UUID `db:"payment_id"`
	CreatedAt time.Time `db:"created_at"`
}

type Challenge struct {
	PaymentID uuid.UUID `db:"payment_id"`
	CreatedAt time.Time `db:"created_at"`
	Nonce     string    `db:"nonce"`
}

func NewChallenge(paymentID uuid.UUID) Challenge {
	return Challenge{
		PaymentID: paymentID,
		CreatedAt: time.Now(),
		Nonce:     base64.URLEncoding.EncodeToString(uuid.NewV4().Bytes()),
	}
}

func (c *Challenge) IsValid(now time.Time) error {
	if c.hasExpired(now) {
		return ErrChallengeExpired
	}
	return nil
}

func (c *Challenge) hasExpired(now time.Time) bool {
	expiresAt := c.CreatedAt.Add(5 * time.Minute)
	return expiresAt.Before(now)
}

type Error string

func (e Error) Error() string {
	return string(e)
}
