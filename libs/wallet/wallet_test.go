package wallet

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfo_LinkSolanaAddress(t *testing.T) {
	type tcGiven struct {
		w Info
		s SolanaLinkReq
	}

	type testCase struct {
		name      string
		given     tcGiven
		assertErr assert.ErrorAssertionFunc
	}

	tests := []testCase{
		{
			name: "success",
			given: tcGiven{
				w: Info{
					ID:        "5e99b1ae-6e91-481f-9021-7fb3c97327d4",
					PublicKey: "f4205e18ea138efc59f18dfbb5c7c2a24b5724395a36e86e84c84b9ec9ccb1ec",
				},
				s: SolanaLinkReq{
					Pub: "GysxUmKWkazQSUm3DCG7o5tPmsuNqFgQ2iG9imkzxzyG",
					Sig: "X2nhxq-95ZR5QWk9R1m-Rqh8QVndDy2yL2NY5PSx0G-EyzX3xm7JKpPhILxZfc_cWwLtaPk6xRQBManPCKE6BQ==",
					Msg: newMsg(msgParts{
						paymentID: "5e99b1ae-6e91-481f-9021-7fb3c97327d4",
						solPub:    "GysxUmKWkazQSUm3DCG7o5tPmsuNqFgQ2iG9imkzxzyG",
						nonce:     "86d6f240-df9b-4167-a66e-5df6da80ac24",
						rewSig:    "szPeTsDRUOFLS1y-k85yfvcI40OOWwTEn2mQ3cGcnZLPkDCXD1qJKYJNkNgdY5j5BA7pvj8AzEy8riKtdeRaAQ==",
					}),
					Nonce: "86d6f240-df9b-4167-a66e-5df6da80ac24",
				},
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
		{
			name: "invalid_linking",
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				var expected *LinkSolanaAddressError
				return assert.ErrorAs(t, err, &expected)
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			wallet := tc.given.w
			err := wallet.LinkSolanaAddress(context.TODO(), tc.given.s)
			tc.assertErr(t, err)
		})
	}
}

func TestVerifySolanaSignature(t *testing.T) {
	type tcGiven struct {
		solPub string
		msg    string
		solSig string
	}

	type testCase struct {
		name      string
		given     tcGiven
		assertErr assert.ErrorAssertionFunc
	}

	tests := []testCase{
		{
			name:  "invalid_public_key_length",
			given: tcGiven{solPub: "123456789"},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errBadPublicKeyLength)
			},
		},
		{
			name:  "signature_has_illegal_character",
			given: tcGiven{solPub: "32rbMEtgTphzVnHuSsuHEv3hKpm92UsgMerjDjZr72T1", solSig: "+"},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error decoding solana signature")
			},
		},
		{
			name: "invalid_signature",
			given: tcGiven{
				solPub: "5zjTAqk1xbeYFzkMCnY3H52SLQpuM1GUFUDAwfhJs1wg",
				msg:    "invalid_message",
				solSig: "zc2boTImAAhzraUplAlUy2L6hNF6l-DYGfOqq_4UfrDsJEBg26jaHIAXJF2i3tifCZxrvmu3ahqIdnm2kOwyBQ==",
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errInvalidSolanaSignature)
			},
		},
		{
			name: "valid_signature",
			given: tcGiven{
				solPub: "5zjTAqk1xbeYFzkMCnY3H52SLQpuM1GUFUDAwfhJs1wg",
				msg:    "test",
				solSig: "zc2boTImAAhzraUplAlUy2L6hNF6l-DYGfOqq_4UfrDsJEBg26jaHIAXJF2i3tifCZxrvmu3ahqIdnm2kOwyBQ==",
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
	}
	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			err := verifySolanaSignature(tc.given.solPub, tc.given.msg, tc.given.solSig)
			tc.assertErr(t, err)
		})
	}
}

func TestSolMsgParser_parse(t *testing.T) {
	type tcGiven struct {
		msgParser solMsgParser
		msg       string
	}

	type exp struct {
		rewMsg rewMsg
		err    error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   exp
	}

	tests := []testCase{
		{
			name: "invalid_number_of_line_breaks",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:payment-id-some-text:solana-address\nsome-text:payment-id.nonce.abcd",
			},
			exp: exp{
				err: errInvalidPartsLineBreak,
			},
		},
		{
			name: "invalid_parts_colon",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "payment-id\nsome-text:solana-address\nsome-text:payment-id.nonce.abcd",
			},
			exp: exp{
				err: errInvalidPartsColon,
			},
		},
		{
			name: "invalid_payment_id",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:another-id\nsome-text:solana-address\nsome-text:payment-id.nonce.abcd",
			},
			exp: exp{
				err: errInvalidPaymentID,
			},
		},
		{
			name: "invalid_solana_public_key",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:payment-id\nsome-text:another-solana-address\nsome-text:payment-id.nonce.abcd",
			},
			exp: exp{
				err: errInvalidSolanaPubKey,
			},
		},
		{
			name: "invalid_rewards_message",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:payment-id\nsome-text:solana-address\nsome-text:another-payment-id.nonce.abcd",
			},
			exp: exp{
				err: errInvalidRewardsMessage,
			},
		},
		{
			name: "no_rewards_message_signature",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:payment-id\nsome-text:solana-address\nsome-text:payment-id.nonce.",
			},
			exp: exp{
				rewMsg: rewMsg{
					msg: "payment-id.nonce",
				},
				err: nil,
			},
		},
		{
			name: "success",
			given: tcGiven{
				msgParser: solMsgParser{paymentID: "payment-id", solPub: "solana-address", nonce: "nonce"},
				msg:       "some-text:payment-id\nsome-text:solana-address\nsome-text:payment-id.nonce.abcd",
			},
			exp: exp{
				rewMsg: rewMsg{
					msg: "payment-id.nonce",
					sig: "abcd",
				},
				err: nil,
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			msgParser := tc.given.msgParser
			actual, err := msgParser.parse(tc.given.msg)
			assert.Equal(t, tc.exp.err, err)
			assert.Equal(t, tc.exp.rewMsg, actual)
		})
	}
}

func TestVerifyRewardsSignature(t *testing.T) {
	type tcGiven struct {
		pub string
		sig string
		msg string
	}

	type testCase struct {
		name      string
		given     tcGiven
		assertErr assert.ErrorAssertionFunc
	}

	tests := []testCase{
		{
			name:  "error_decoding_public_key",
			given: tcGiven{pub: "invalid_key"},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error decoding rewards public key")
			},
		},
		{
			name:  "invalid_public_key_length",
			given: tcGiven{pub: hex.EncodeToString([]byte("key"))},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errBadPublicKeyLength)
			},
		},
		{
			name: "signature_has_illegal_character",
			given: tcGiven{
				pub: "ac1e69da621a99cf29de8ac1b0ffc8ece154b98e99a0ebec2bfdf2af04b8ac53",
				sig: "!",
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorContains(t, err, "error decoding rewards signature")
			},
		},
		{
			name: "invalid_signature",
			given: tcGiven{
				pub: "e0e9196cfb3c98f8912c011ff46193167b7df72a166c595408c6ca6c690bb707",
				msg: "invalid_message",
				sig: "gJJptSk0lGBjpJOx7Mq_AwVtNkW5tg4esgbtYesQXLfabDZP4K_bFxpEn40TIBRISQho9oLzGfOnzWH88ntdAg=="},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.ErrorIs(t, err, errInvalidRewardsSignature)
			},
		},
		{
			name: "valid_signature",
			given: tcGiven{
				pub: "e0e9196cfb3c98f8912c011ff46193167b7df72a166c595408c6ca6c690bb707",
				msg: "test",
				sig: "gJJptSk0lGBjpJOx7Mq_AwVtNkW5tg4esgbtYesQXLfabDZP4K_bFxpEn40TIBRISQho9oLzGfOnzWH88ntdAg==",
			},
			assertErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				return assert.NoError(t, err)
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			err := verifyRewardsSignature(tc.given.pub, tc.given.msg, tc.given.sig)
			tc.assertErr(t, err)
		})
	}
}

func TestNewLinkSolanaAddressError(t *testing.T) {
	err1 := errors.New("error text")
	err2 := newLinkSolanaAddressError(err1)
	err3 := fmt.Errorf("%w", err2)

	t.Run("error_text", func(t *testing.T) {
		assert.ErrorContains(t, err2, err1.Error())
	})

	t.Run("error_unwrap", func(t *testing.T) {
		var target *LinkSolanaAddressError
		assert.ErrorAs(t, err3, &target)
	})
}

type msgParts struct {
	paymentID string
	solPub    string
	nonce     string
	rewSig    string
}

func newMsg(parts msgParts) string {
	const msgTmpl = "<some-text>:%s\n<some-text>:%s\n<some-text>:%s.%s.%s"
	return fmt.Sprintf(msgTmpl, parts.paymentID, parts.solPub, parts.paymentID, parts.nonce, parts.rewSig)
}
