package logging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	uuid "github.com/satori/go.uuid"
)

func TestAddWalletIDToContext(t *testing.T) {
	type logLine struct {
		WalletID uuid.UUID `json:"walletID"`
	}

	var b bytes.Buffer
	output := bufio.NewWriter(&b)

	log := zerolog.New(output).With().Timestamp().Logger()
	ctx := log.WithContext(context.Background())

	walletID := uuid.NewV4()

	AddWalletIDToContext(ctx, walletID)

	l := zerolog.Ctx(ctx)
	l.Debug().Msg("test")
	err := output.Flush()
	if err != nil {
		t.Fatal(err)
	}

	var line logLine
	err = json.Unmarshal(b.Bytes(), &line)
	if err != nil {
		t.Fatal(err)
	}

	if !uuid.Equal(line.WalletID, walletID) {
		t.Fatal("WalletID must be included")
	}
}
