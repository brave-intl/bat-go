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

func TestAddPaymentIDToContext(t *testing.T) {
	type logLine struct {
		PaymentID uuid.UUID `json:"paymentID"`
	}

	var b bytes.Buffer
	output := bufio.NewWriter(&b)

	log := zerolog.New(output).With().Timestamp().Logger()
	ctx := log.WithContext(context.Background())

	paymentID := uuid.NewV4()

	AddPaymentIDToContext(ctx, paymentID)

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

	if !uuid.Equal(line.PaymentID, paymentID) {
		t.Fatal("PaymentID must be included")
	}
}
