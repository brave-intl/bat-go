package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/brave-intl/bat-go/utils/ptr"

	"github.com/brave-intl/bat-go/utils/clients/bitflyer"
	mock_bitflyer "github.com/brave-intl/bat-go/utils/clients/bitflyer/mock"
	errorutils "github.com/brave-intl/bat-go/utils/errors"
	"github.com/brave-intl/bat-go/utils/logging"
	"github.com/shopspring/decimal"

	kafkautils "github.com/brave-intl/bat-go/utils/kafka"
	"github.com/golang/mock/gomock"
	"github.com/linkedin/goavro"
	uuid "github.com/satori/go.uuid"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMessage_KafkaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kafkaReader := NewMockKafkaReader(ctrl)

	ctx := context.Background()
	err := errors.New(uuid.NewV4().String())

	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(kafka.Message{}, err)

	s := Service{
		kafkaAdminAttestationReader: kafkaReader,
	}

	expected := fmt.Errorf("read message: error reading kafka message %w", err)

	walletID, actual := s.FetchAdminAttestationWalletID(ctx)

	assert.Nil(t, walletID)
	assert.EqualError(t, actual, expected.Error())
}

func TestReadMessage_CodecError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kafkaReader := NewMockKafkaReader(ctrl)

	ctx := context.Background()

	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(kafka.Message{}, nil)

	codec := make(map[string]*goavro.Codec)

	s := Service{
		codecs:                      codec,
		kafkaAdminAttestationReader: kafkaReader,
	}

	expected := fmt.Errorf("read message: could not find codec %s", adminAttestationTopic)

	walletID, actual := s.FetchAdminAttestationWalletID(ctx)

	assert.Nil(t, walletID)
	assert.EqualError(t, actual, expected.Error())
}

func TestReadMessage_WalletIDInvalidError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	codecs, err := kafkautils.GenerateCodecs(map[string]string{
		adminAttestationTopic: adminAttestationEventSchema,
	})
	require.NoError(t, err)

	ctx := context.Background()
	msg := makeMsg()

	msg.WalletID = "invalid"

	textual, err := json.Marshal(msg)
	require.NoError(t, err)

	native, _, err := codecs[adminAttestationTopic].NativeFromTextual(textual)
	require.NoError(t, err)

	binary, err := codecs[adminAttestationTopic].BinaryFromNative(nil, native)
	require.NoError(t, err)

	message := kafka.Message{
		Key:   []byte(uuid.NewV4().String()),
		Value: binary,
	}

	kafkaReader := NewMockKafkaReader(ctrl)
	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(message, nil)

	s := Service{
		codecs:                      codecs,
		kafkaAdminAttestationReader: kafkaReader,
	}

	expected := fmt.Errorf("read message: error could not decode walletID %s", msg.WalletID)

	walletID, actual := s.FetchAdminAttestationWalletID(ctx)
	require.NoError(t, err)

	assert.Nil(t, walletID)
	assert.EqualError(t, actual, expected.Error())
}

func TestReadMessage_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	codecs, err := kafkautils.GenerateCodecs(map[string]string{
		adminAttestationTopic: adminAttestationEventSchema,
	})
	require.NoError(t, err)

	ctx := context.Background()
	msg := makeMsg()

	textual, err := json.Marshal(msg)
	require.NoError(t, err)

	native, _, err := codecs[adminAttestationTopic].NativeFromTextual(textual)
	require.NoError(t, err)

	binary, err := codecs[adminAttestationTopic].BinaryFromNative(nil, native)
	require.NoError(t, err)

	message := kafka.Message{
		Key:   []byte(uuid.NewV4().String()),
		Value: binary,
	}

	kafkaReader := NewMockKafkaReader(ctrl)
	kafkaReader.EXPECT().
		ReadMessage(gomock.Eq(ctx)).
		Return(message, nil)

	s := Service{
		codecs:                      codecs,
		kafkaAdminAttestationReader: kafkaReader,
	}

	expected, err := uuid.FromString(msg.WalletID)
	require.NoError(t, err)

	actual, err := s.FetchAdminAttestationWalletID(ctx)
	require.NoError(t, err)

	assert.Equal(t, &expected, actual)
}

func TestSubmitBatchTransfer_Nil_DepositDestination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := ptr.FromUUID(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bitFlyerClient := mock_bitflyer.NewMockClient(ctrl)
	bitFlyerClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 5)

	for i := 0; i < len(drainTransfers); i++ {
		depositID := ptr.FromString(uuid.NewV4().String())
		// set invalid deposit id
		if i == 3 {
			depositID = nil
		}
		drainTransfers[i] = DrainTransfer{
			ID:        ptr.FromUUID(uuid.NewV4()),
			Total:     decimal.NewFromFloat(1),
			DepositID: depositID,
		}
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	s := Service{
		bfClient:  bitFlyerClient,
		Datastore: datastore,
	}

	expected := errorutils.New(fmt.Errorf("failed depositID cannot be nil for batchID %s", batchID),
		"submit batch transfer", drainCodeErrorInvalidDepositID)

	err := s.SubmitBatchTransfer(ctx, batchID)
	assert.Equal(t, expected, err)
}

func TestGetGeminiTxnStatus_Completed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	txRef := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Ok",
		Status: ptr.FromString("Completed"),
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, txRef).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	actual, err := service.GetGeminiTxnStatus(ctx, txRef)

	assert.Nil(t, err)
	assert.Equal(t, "complete", actual.Status)
	assert.Equal(t, "", actual.Note)
}
func TestGetGeminiTxnStatus_ClientError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	transactionID := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	clientError := fmt.Errorf(testutils.RandomString())

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, transactionID).
		Return(nil, clientError)

	service := Service{
		geminiClient: geminiClient,
	}

	expected := fmt.Errorf("failed to check gemini txn status for %s: %w", transactionID, clientError)

	status, actual := service.GetGeminiTxnStatus(ctx, transactionID)

	assert.Nil(t, status)
	assert.Equal(t, expected, actual)
}

func TestGetGeminiTxnStatus_ResponseError_Reason(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	transactionID := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Error",
		Reason: ptr.FromString(testutils.RandomString()),
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, transactionID).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	expected := fmt.Errorf("failed to get gemini txn status for %s: %s", transactionID, *response.Reason)

	status, actual := service.GetGeminiTxnStatus(ctx, transactionID)

	assert.Nil(t, status)
	assert.Equal(t, expected, actual)
}

func TestGetGeminiTxnStatus_ResponseError_Nil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	transactionID := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Error",
		Reason: nil,
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, transactionID).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	expected := fmt.Errorf("failed to get gemini txn status for %s: %s", transactionID, "response error")

	status, actual := service.GetGeminiTxnStatus(ctx, transactionID)

	assert.Nil(t, status)
	assert.Equal(t, expected, actual)
}

func TestGetGeminiTxnStatus_Pending(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	txRef := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Ok",
		Status: ptr.FromString("Pending"),
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, txRef).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	actual, err := service.GetGeminiTxnStatus(ctx, txRef)

	assert.Nil(t, err)
	assert.Equal(t, "pending", actual.Status)
	assert.Equal(t, "", actual.Note)
}

func TestGetGeminiTxnStatus_Failed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	txRef := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Ok",
		Status: ptr.FromString("Failed"),
		Reason: ptr.FromString(testutils.RandomString()),
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, txRef).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	actual, err := service.GetGeminiTxnStatus(ctx, txRef)

	assert.Nil(t, err)
	assert.Equal(t, "failed", actual.Status)
	assert.Equal(t, *response.Reason, actual.Note)
}

func TestGetGeminiTxnStatus_Unknown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	txRef := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Ok",
		Status: ptr.FromString(testutils.RandomString()),
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, txRef).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	txStatus, err := service.GetGeminiTxnStatus(ctx, txRef)

	assert.Nil(t, txStatus)
	assert.Error(t, err, fmt.Errorf("failed to get txn status for %s: unknown status %s",
		txRef, ptr.String(response.Status)).Error())
}

func TestGetGeminiTxnStatus_ResponseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	apiKey := testutils.RandomString()
	clientID := testutils.RandomString()
	txRef := testutils.RandomString()

	ctx := context.Background()
	ctx = context.WithValue(ctx, appctx.GeminiAPIKeyCTXKey, apiKey)
	ctx = context.WithValue(ctx, appctx.GeminiClientIDCTXKey, clientID)

	response := &gemini.PayoutResult{
		Result: "Error",
	}

	geminiClient := mock_gemini.NewMockClient(ctrl)
	geminiClient.EXPECT().
		CheckTxStatus(ctx, apiKey, clientID, txRef).
		Return(response, nil)

	service := Service{
		geminiClient: geminiClient,
	}

	txStatus, err := service.GetGeminiTxnStatus(ctx, txRef)

	assert.Nil(t, txStatus)
	assert.Errorf(t, err, fmt.Errorf("failed to get gemini txn status for %s", txRef).Error())
}

func TestSubmitBatchTransfer_UploadBulkPayout_NOINV(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := uuidToPointer(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bfClient := mock_bitflyer.NewMockClient(ctrl)
	bfClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 1)
	drainTransfers[0] = DrainTransfer{
		ID:        uuidToPointer(uuid.NewV4()),
		Total:     decimal.NewFromFloat(1),
		DepositID: stringToPointer(uuid.NewV4().String()),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	var bitflyerError = new(clients.BitflyerError)
	bitflyerError.HTTPStatusCode = http.StatusUnauthorized

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(nil, bitflyerError)

	bfClient.EXPECT().
		RefreshToken(ctx, gomock.Any()).
		Return(nil, nil)

	withdrawal := bitflyer.WithdrawToDepositIDResponse{
		Status: "NO_INV",
	}

	withdrawToDepositIDBulkResponse := bitflyer.WithdrawToDepositIDBulkResponse{
		DryRun: false,
		Withdrawals: []bitflyer.WithdrawToDepositIDResponse{
			withdrawal,
		},
	}

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(&withdrawToDepositIDBulkResponse, nil)

	s := Service{
		bfClient:  bfClient,
		Datastore: datastore,
	}

	err := fmt.Errorf("submit batch transfer error: bitflyer %s error for batchID %s",
		withdrawal.Status, withdrawal.TransferID)

	codified := errorutils.Codified{
		ErrCode: "bitflyer_no_inv",
		Retry:   false,
	}

	expected := errorutils.New(err, "submit batch transfer", codified)
	actual := s.SubmitBatchTransfer(ctx, batchID)

	assert.Equal(t, expected, actual)
}

func TestSubmitBatchTransfer_UploadBulkPayout_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := uuidToPointer(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bfClient := mock_bitflyer.NewMockClient(ctrl)
	bfClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 1)
	drainTransfers[0] = DrainTransfer{
		ID:        uuidToPointer(uuid.NewV4()),
		Total:     decimal.NewFromFloat(1),
		DepositID: stringToPointer(uuid.NewV4().String()),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	var bitflyerError = new(clients.BitflyerError)
	bitflyerError.HTTPStatusCode = http.StatusUnauthorized

	err := errors.New("some error")

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(nil, err)

	s := Service{
		bfClient:  bfClient,
		Datastore: datastore,
	}

	actual := s.SubmitBatchTransfer(ctx, batchID)

	assert.Errorf(t, actual, "failed to transfer funds: %s", err.Error())
}

func TestSubmitBatchTransfer_UploadBulkPayout_Bitflyer_Unauthorized_Retry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := uuidToPointer(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bfClient := mock_bitflyer.NewMockClient(ctrl)
	bfClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 1)
	drainTransfers[0] = DrainTransfer{
		ID:        uuidToPointer(uuid.NewV4()),
		Total:     decimal.NewFromFloat(1),
		DepositID: stringToPointer(uuid.NewV4().String()),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	var bitflyerError = new(clients.BitflyerError)
	bitflyerError.HTTPStatusCode = http.StatusUnauthorized

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(nil, bitflyerError)

	bfClient.EXPECT().
		RefreshToken(ctx, gomock.Any()).
		Return(nil, nil)

	withdrawToDepositIDBulkResponse := bitflyer.WithdrawToDepositIDBulkResponse{
		DryRun: false,
		Withdrawals: []bitflyer.WithdrawToDepositIDResponse{
			{
				Status: "SUCCESS",
			},
		},
	}

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(&withdrawToDepositIDBulkResponse, nil)

	datastore.EXPECT().
		MarkBatchTransferSubmitted(ctx, batchID).
		Return(nil)

	s := Service{
		bfClient:  bfClient,
		Datastore: datastore,
	}

	err := s.SubmitBatchTransfer(ctx, batchID)
	assert.Nil(t, err)
}

func TestSubmitBatchTransfer_UploadBulkPayout_Bitflyer_Unauthorized_NoRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := uuidToPointer(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bfClient := mock_bitflyer.NewMockClient(ctrl)
	bfClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 1)
	drainTransfers[0] = DrainTransfer{
		ID:        uuidToPointer(uuid.NewV4()),
		Total:     decimal.NewFromFloat(1),
		DepositID: stringToPointer(uuid.NewV4().String()),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	var bitflyerError = new(clients.BitflyerError)
	bitflyerError.HTTPStatusCode = http.StatusUnauthorized

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(nil, bitflyerError)

	refreshTokenError := errors.New("some error")
	bfClient.EXPECT().
		RefreshToken(ctx, gomock.Any()).
		Return(nil, refreshTokenError)

	s := Service{
		bfClient:  bfClient,
		Datastore: datastore,
	}

	err := s.SubmitBatchTransfer(ctx, batchID)

	assert.Errorf(t, err, "failed to get token from bf: %s", refreshTokenError.Error())
}

func TestSubmitBatchTransfer_UploadBulkPayout_Bitflyer_NoWithdrawals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, _ := logging.SetupLogger(context.Background())
	batchID := uuidToPointer(uuid.NewV4())

	quote := bitflyer.Quote{
		Rate: decimal.New(1, 1),
	}

	bfClient := mock_bitflyer.NewMockClient(ctrl)
	bfClient.EXPECT().
		FetchQuote(ctx, "BAT_JPY", false).
		Return(&quote, nil)

	drainTransfers := make([]DrainTransfer, 1)
	drainTransfers[0] = DrainTransfer{
		ID:        uuidToPointer(uuid.NewV4()),
		Total:     decimal.NewFromFloat(1),
		DepositID: stringToPointer(uuid.NewV4().String()),
	}

	datastore := NewMockDatastore(ctrl)
	datastore.EXPECT().
		GetDrainsByBatchID(ctx, batchID).
		Return(drainTransfers, nil)

	var bitflyerError = new(clients.BitflyerError)
	bitflyerError.HTTPStatusCode = http.StatusUnauthorized

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(nil, bitflyerError)

	bfClient.EXPECT().
		RefreshToken(ctx, gomock.Any()).
		Return(nil, nil)

	// no withdraws
	withdrawToDepositIDBulkResponse := bitflyer.WithdrawToDepositIDBulkResponse{
		DryRun:      false,
		Withdrawals: []bitflyer.WithdrawToDepositIDResponse{},
	}

	bfClient.EXPECT().
		UploadBulkPayout(ctx, gomock.Any()).
		Return(&withdrawToDepositIDBulkResponse, nil)

	s := Service{
		bfClient:  bfClient,
		Datastore: datastore,
	}

	err := s.SubmitBatchTransfer(ctx, batchID)

	assert.Errorf(t, err, "submit batch transfer error: response cannot be nil for batchID %s", batchID)
}

func stringToPointer(s string) *string {
	return &s
}

func uuidToPointer(u uuid.UUID) *uuid.UUID {
	return &u
}

func makeMsg() AdminAttestationEvent {
	return AdminAttestationEvent{
		WalletID:      uuid.NewV4().String(),
		Service:       uuid.NewV4().String(),
		Signal:        uuid.NewV4().String(),
		Score:         rand.Int31n(10),
		Justification: uuid.NewV4().String(),
		CreatedAt:     time.Now().String(),
	}
}
