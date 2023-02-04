package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type confResp struct {
	PublicKey string `json:"publicKey"`
}

// GetPaymentsPubKey - get the public key from the payments server
func GetPaymentsPubKey(ctx context.Context, paymentsHost string) (ed25519.PublicKey, error) {
	// the public key is on the payments service in configuration
	resp, err := http.Get(fmt.Sprintf("%s/v1/configuration", paymentsHost))
	if err != nil {
		return nil, LogAndError(ctx, err, "GetPaymentsPubKey", "failed to build request to get pub key")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, LogAndError(ctx, err, "GetPaymentsPubKey", "failed to read response from payments")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, LogAndError(
			ctx, fmt.Errorf(
				"%d - %s",
				resp.StatusCode, string(body),
			), "GetPaymentsPubKey", "invalid response from server")
	}

	var conf = new(confResp)
	// parse the json response
	if err := json.Unmarshal(body, conf); err != nil {
		return nil, LogAndError(ctx, err, "GetPaymentsPubKey", "invalid json from server")
	}

	// convert the hex encoded public key into an ed25519.PublicKey
	data, err := hex.DecodeString(conf.PublicKey)
	if err != nil {
		return nil, LogAndError(ctx, err, "GetPaymentsPubKey", "invalid public key from server")
	}

	return ed25519.PublicKey(data), nil
}
