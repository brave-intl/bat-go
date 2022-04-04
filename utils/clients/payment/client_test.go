package payment

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/brave-intl/bat-go/utils/httpsignature"
	"github.com/brave-intl/bat-go/utils/ptr"
	testutils "github.com/brave-intl/bat-go/utils/test"
	uuid "github.com/satori/go.uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepare(t *testing.T) {
	expected := make([]Transaction, 5)
	for i := 0; i < 5; i++ {
		expected[i] = Transaction{
			IdempotencyKey: uuid.NewV4(),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
		}
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/payments/prepare", r.URL.Path)

		// assert we received the expected transactions
		var transactions []Transaction
		err := json.NewDecoder(r.Body).Decode(&transactions)

		require.NoError(t, err)
		assert.Equal(t, expected, transactions)

		// return the received transactions
		w.WriteHeader(http.StatusCreated)

		payload, err := json.Marshal(transactions)
		require.NoError(t, err)

		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	defer ts.Close()

	client := New(ts.URL, httpsignature.ParameterizedSignator{})
	actual, err := client.Prepare(context.Background(), expected)
	assert.Nil(t, err)

	assert.Equal(t, expected, *actual)
}

func TestSubmit(t *testing.T) {
	expected := make([]Transaction, 5)
	for i := 0; i < 5; i++ {
		expected[i] = Transaction{
			IdempotencyKey: uuid.NewV4(),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
		}
	}

	keyID := uuid.NewV4().String()

	key, privateKey, err := httpsignature.GenerateEd25519Key(nil)
	assert.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/payments/submit", r.URL.Path)

		sp := httpsignature.SignatureParams{
			KeyID:     keyID,
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		}

		// check headers
		assert.NotEmpty(t, r.Header["Digest"])
		assert.NotEmpty(t, r.Header["Signature"])

		valid, err := sp.Verify(key, crypto.Hash(0), r)
		assert.NoError(t, err)
		assert.True(t, valid)

		// assert we received the expected transactions
		var transactions []Transaction
		err = json.NewDecoder(r.Body).Decode(&transactions)

		require.NoError(t, err)
		assert.Equal(t, expected, transactions)

		// return the received transactions
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	pc := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{
			KeyID:     keyID,
			Algorithm: httpsignature.ED25519,
			Headers:   []string{"digest", "(request-target)"},
		},
		Signator: privateKey,
		Opts:     crypto.Hash(0),
	}

	client := New(ts.URL, pc)
	err = client.Submit(context.Background(), expected)
	assert.Nil(t, err)
}

func TestSubmit_SignatureError(t *testing.T) {
	expected := []Transaction{
		{IdempotencyKey: uuid.NewV4(),
			Amount: decimal.New(1, 0),
			To:     uuid.NewV4(),
			From:   uuid.NewV4(),
		},
	}

	// cause signing error
	pc := httpsignature.ParameterizedSignator{
		SignatureParams: httpsignature.SignatureParams{},
	}

	client := New(testutils.RandomString(), pc)
	err := client.Submit(context.Background(), expected)

	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "error signing http request")
}

func TestStatus(t *testing.T) {
	documentID := uuid.NewV4().String()
	expected := TransactionStatus{
		CustodianSubmissionResponse: ptr.FromString(testutils.RandomString()),
		CustodianStatusResponse:     ptr.FromString(testutils.RandomString()),
		Transaction: Transaction{
			IdempotencyKey: uuid.NewV4(),
			Custodian:      ptr.FromString(testutils.RandomString()),
			Amount:         decimal.New(1, 0),
			To:             uuid.NewV4(),
			From:           uuid.NewV4(),
			DocumentID:     ptr.FromString(documentID),
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, fmt.Sprintf("/v1/payments/%s/status", documentID), r.URL.Path)

		w.WriteHeader(http.StatusOK)

		payload, err := json.Marshal(expected)
		assert.NoError(t, err)

		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	defer ts.Close()

	client := New(ts.URL, httpsignature.ParameterizedSignator{})
	actual, err := client.Status(context.Background(), documentID)
	assert.Nil(t, err)

	assert.Equal(t, expected.CustodianSubmissionResponse, actual.CustodianSubmissionResponse)
	assert.Equal(t, expected.CustodianStatusResponse, actual.CustodianStatusResponse)
	assert.Equal(t, expected.Transaction, actual.Transaction)
}

func TestUnwrapPaymentError(t *testing.T) {
	type CustodianError struct {
		Field string `json:"field"`
	}

	expected := Error{
		Code:    testutils.RandomInt(),
		Message: testutils.RandomString(),
		Data: CustodianError{
			Field: testutils.RandomString(),
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)

		payload, err := json.Marshal(expected)
		assert.NoError(t, err)

		_, err = w.Write(payload)
		assert.NoError(t, err)
	}))
	defer ts.Close()

	client := New(ts.URL, httpsignature.ParameterizedSignator{})
	res, err := client.Status(context.Background(), uuid.NewV4().String())
	assert.Nil(t, res)
	assert.NotNil(t, err)

	actual, err := UnwrapPaymentError(err)
	assert.NoError(t, err)

	assert.Equal(t, expected.Code, actual.Code)
	assert.Equal(t, expected.Message, actual.Message)

	data, err := json.Marshal(actual.Data)
	assert.NoError(t, err)

	var custodianError CustodianError
	err = json.Unmarshal(data, &custodianError)
	assert.NoError(t, err)

	assert.Equal(t, expected.Data.(CustodianError).Field, custodianError.Field)
}
