package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

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
