package payments

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/brave-intl/bat-go/libs/custodian/provider"
	"github.com/brave-intl/bat-go/libs/service"
	"testing"

	"github.com/amazon-ion/ion-go/ion"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyKeyGeneration(t *testing.T) {
	to, err := uuid.Parse("")
	from, err := uuid.Parse("")
	if err != nil {
		panic("failed to parse test UUIDs")
	}
	transaction := Transaction{
		IdempotencyKey:      "",
		Amount:              ion.MustParseDecimal("12.234"),
		To:                  &to,
		From:                &from,
		Custodian:           "uphold",
		State:               Initialized,
		DocumentID:          "1234",
		AttestationDocument: "4567",
		PayoutID:            "78910",
		Signature:           "",
		PublicKey:           "",
	}
	assert.Equal(t, transaction.deriveIdempotencyKey(), "")
}

func TestService_AuthorizeTransaction(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx         context.Context
		keyID       string
		transaction Transaction
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			tt.wantErr(t, s.AuthorizeTransaction(tt.args.ctx, tt.args.keyID, tt.args.transaction), fmt.Sprintf("AuthorizeTransaction(%v, %v, %v)", tt.args.ctx, tt.args.keyID, tt.args.transaction))
		})
	}
}

func TestService_GetTransactionFromDocID(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx   context.Context
		docID string
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		want    *Transaction
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			got, err := s.GetTransactionFromDocID(tt.args.ctx, tt.args.docID)
			if !tt.wantErr(t, err, fmt.Sprintf("GetTransactionFromDocID(%v, %v)", tt.args.ctx, tt.args.docID)) {
				return
			}
			assert.Equalf(t, tt.want, got, "GetTransactionFromDocID(%v, %v)", tt.args.ctx, tt.args.docID)
		})
	}
}

func TestService_InsertTransaction(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx         context.Context
		transaction *Transaction
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		want    Transaction
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			got, err := s.InsertTransaction(tt.args.ctx, tt.args.transaction)
			if !tt.wantErr(t, err, fmt.Sprintf("InsertTransaction(%v, %v)", tt.args.ctx, tt.args.transaction)) {
				return
			}
			assert.Equalf(t, tt.want, got, "InsertTransaction(%v, %v)", tt.args.ctx, tt.args.transaction)
		})
	}
}

func TestService_UpdateTransactionsState(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx          context.Context
		state        string
		transactions []Transaction
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			tt.wantErr(t, s.UpdateTransactionsState(tt.args.ctx, tt.args.state, tt.args.transactions...), fmt.Sprintf(
				"UpdateTransactionsState(%v, %v, %v)",
				tt.args.ctx,
				tt.args.state,
				tt.args.transactions,
			))
		})
	}
}

func TestService_configureDatastore(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx context.Context
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			tt.wantErr(t, s.configureDatastore(tt.args.ctx), fmt.Sprintf("configureDatastore(%v)", tt.args.ctx))
		})
	}
}

func TestService_setupLedger(t *testing.T) {
	type fields struct {
		datastore        wrappedQldbDriverAPI
		custodians       map[string]provider.Custodian
		baseCtx          context.Context
		secretMgr        service.SecretManager
		keyShares        [][]byte
		kmsDecryptKeyArn string
		kmsSigningKeyId  string
		kmsSigningClient *kms.Client
	}
	type args struct {
		ctx context.Context
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				datastore:        tt.fields.datastore,
				custodians:       tt.fields.custodians,
				baseCtx:          tt.fields.baseCtx,
				secretMgr:        tt.fields.secretMgr,
				keyShares:        tt.fields.keyShares,
				kmsDecryptKeyArn: tt.fields.kmsDecryptKeyArn,
				kmsSigningKeyId:  tt.fields.kmsSigningKeyId,
				kmsSigningClient: tt.fields.kmsSigningClient,
			}
			tt.wantErr(t, s.setupLedger(tt.args.ctx), fmt.Sprintf("setupLedger(%v)", tt.args.ctx))
		})
	}
}

func TestTransaction_BuildSigningBytes(t1 *testing.T) {
	type fields struct {
		IdempotencyKey      string
		Amount              *ion.Decimal
		To                  *uuid.UUID
		From                *uuid.UUID
		Custodian           string
		State               TransactionState
		DocumentID          string
		AttestationDocument string
		PayoutID            string
		Signature           string
		PublicKey           string
	}
	var tests []struct {
		name   string
		fields fields
		want   []byte
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := Transaction{
				IdempotencyKey:      tt.fields.IdempotencyKey,
				Amount:              tt.fields.Amount,
				To:                  tt.fields.To,
				From:                tt.fields.From,
				Custodian:           tt.fields.Custodian,
				State:               tt.fields.State,
				DocumentID:          tt.fields.DocumentID,
				AttestationDocument: tt.fields.AttestationDocument,
				PayoutID:            tt.fields.PayoutID,
				Signature:           tt.fields.Signature,
				PublicKey:           tt.fields.PublicKey,
			}
			assert.Equalf(t1, tt.want, t.BuildSigningBytes(), "BuildSigningBytes()")
		})
	}
}

func TestTransaction_MarshalJSON(t1 *testing.T) {
	type fields struct {
		IdempotencyKey      string
		Amount              *ion.Decimal
		To                  *uuid.UUID
		From                *uuid.UUID
		Custodian           string
		State               TransactionState
		DocumentID          string
		AttestationDocument string
		PayoutID            string
		Signature           string
		PublicKey           string
	}
	var tests []struct {
		name    string
		fields  fields
		want    []byte
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				IdempotencyKey:      tt.fields.IdempotencyKey,
				Amount:              tt.fields.Amount,
				To:                  tt.fields.To,
				From:                tt.fields.From,
				Custodian:           tt.fields.Custodian,
				State:               tt.fields.State,
				DocumentID:          tt.fields.DocumentID,
				AttestationDocument: tt.fields.AttestationDocument,
				PayoutID:            tt.fields.PayoutID,
				Signature:           tt.fields.Signature,
				PublicKey:           tt.fields.PublicKey,
			}
			got, err := t.MarshalJSON()
			if !tt.wantErr(t1, err, fmt.Sprintf("MarshalJSON()")) {
				return
			}
			assert.Equalf(t1, tt.want, got, "MarshalJSON()")
		})
	}
}

func TestTransaction_SignTransaction(t1 *testing.T) {
	type fields struct {
		IdempotencyKey      string
		Amount              *ion.Decimal
		To                  *uuid.UUID
		From                *uuid.UUID
		Custodian           string
		State               TransactionState
		DocumentID          string
		AttestationDocument string
		PayoutID            string
		Signature           string
		PublicKey           string
	}
	type args struct {
		ctx       context.Context
		kmsClient *kms.Client
		keyId     string
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		want    string
		want1   string
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				IdempotencyKey:      tt.fields.IdempotencyKey,
				Amount:              tt.fields.Amount,
				To:                  tt.fields.To,
				From:                tt.fields.From,
				Custodian:           tt.fields.Custodian,
				State:               tt.fields.State,
				DocumentID:          tt.fields.DocumentID,
				AttestationDocument: tt.fields.AttestationDocument,
				PayoutID:            tt.fields.PayoutID,
				Signature:           tt.fields.Signature,
				PublicKey:           tt.fields.PublicKey,
			}
			got, got1, err := t.SignTransaction(tt.args.ctx, tt.args.kmsClient, tt.args.keyId)
			if !tt.wantErr(t1, err, fmt.Sprintf("SignTransaction(%v, %v, %v)", tt.args.ctx, tt.args.kmsClient, tt.args.keyId)) {
				return
			}
			assert.Equalf(t1, tt.want, got, "SignTransaction(%v, %v, %v)", tt.args.ctx, tt.args.kmsClient, tt.args.keyId)
			assert.Equalf(t1, tt.want1, got1, "SignTransaction(%v, %v, %v)", tt.args.ctx, tt.args.kmsClient, tt.args.keyId)
		})
	}
}

func TestTransaction_UnmarshalJSON(t1 *testing.T) {
	type fields struct {
		IdempotencyKey      string
		Amount              *ion.Decimal
		To                  *uuid.UUID
		From                *uuid.UUID
		Custodian           string
		State               TransactionState
		DocumentID          string
		AttestationDocument string
		PayoutID            string
		Signature           string
		PublicKey           string
	}
	type args struct {
		data []byte
	}
	var tests []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				IdempotencyKey:      tt.fields.IdempotencyKey,
				Amount:              tt.fields.Amount,
				To:                  tt.fields.To,
				From:                tt.fields.From,
				Custodian:           tt.fields.Custodian,
				State:               tt.fields.State,
				DocumentID:          tt.fields.DocumentID,
				AttestationDocument: tt.fields.AttestationDocument,
				PayoutID:            tt.fields.PayoutID,
				Signature:           tt.fields.Signature,
				PublicKey:           tt.fields.PublicKey,
			}
			tt.wantErr(t1, t.UnmarshalJSON(tt.args.data), fmt.Sprintf("UnmarshalJSON(%v)", tt.args.data))
		})
	}
}

func TestTransaction_deriveIdempotencyKey(t1 *testing.T) {
	type fields struct {
		IdempotencyKey      string
		Amount              *ion.Decimal
		To                  *uuid.UUID
		From                *uuid.UUID
		Custodian           string
		State               TransactionState
		DocumentID          string
		AttestationDocument string
		PayoutID            string
		Signature           string
		PublicKey           string
	}
	var tests []struct {
		name   string
		fields fields
		want   string
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &Transaction{
				IdempotencyKey:      tt.fields.IdempotencyKey,
				Amount:              tt.fields.Amount,
				To:                  tt.fields.To,
				From:                tt.fields.From,
				Custodian:           tt.fields.Custodian,
				State:               tt.fields.State,
				DocumentID:          tt.fields.DocumentID,
				AttestationDocument: tt.fields.AttestationDocument,
				PayoutID:            tt.fields.PayoutID,
				Signature:           tt.fields.Signature,
				PublicKey:           tt.fields.PublicKey,
			}
			assert.Equalf(t1, tt.want, t.deriveIdempotencyKey(), "deriveIdempotencyKey()")
		})
	}
}

func Test_fromIonDecimal(t *testing.T) {
	type args struct {
		v *ion.Decimal
	}
	var tests []struct {
		name string
		args args
		want *decimal.Decimal
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, fromIonDecimal(tt.args.v), "fromIonDecimal(%v)", tt.args.v)
		})
	}
}

func Test_idempotencyKeyIsValid(t *testing.T) {
	type args struct {
		txn   *Transaction
		entry *QLDBPaymentTransitionHistoryEntry
	}
	var tests []struct {
		name string
		args args
		want bool
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, idempotencyKeyIsValid(tt.args.txn, tt.args.entry), "idempotencyKeyIsValid(%v, %v)", tt.args.txn, tt.args.entry)
		})
	}
}

func Test_toIonDecimal(t *testing.T) {
	type args struct {
		v *decimal.Decimal
	}
	var tests []struct {
		name string
		args args
		want *ion.Decimal
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, toIonDecimal(tt.args.v), "toIonDecimal(%v)", tt.args.v)
		})
	}
}
