package main

import (
	"encoding/json"
	"testing"
)

func TestParsing(t *testing.T) {
	sesNotificationJSON := "{\"eventType\":\"Delivery\",\"mail\":{\"messageId\":\"010101837a8465ac-5ac03c49-d6bc-426a-907e-1378e1e6831c-000000\",\"tags\":{\"ses:operation\":[\"SendTemplatedEmail\"],\"idempotencyKey\":[\"b968f271-24f7-4f57-84cf-6713a9823154\"]}},\"delivery\":{\"timestamp\":\"2022-09-26T15:57:21.861Z\"}}\n"
	var notification sesNotification
	err := json.Unmarshal([]byte(sesNotificationJSON), &notification)
	if err != nil {
		t.Error("Unable to unmarshal sesNotification:", err)
	}

	if notification.Mail.Tags["idempotencyKey"][0] != "b968f271-24f7-4f57-84cf-6713a9823154" {
		t.Errorf("Idempotency key is incorrect")
	}
}
