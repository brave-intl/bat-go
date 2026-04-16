package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	rootcmd "github.com/brave-intl/bat-go/cmd"
	"github.com/brave-intl/bat-go/services/skus"
	"github.com/brave-intl/bat-go/services/skus/model"
)

var SkusCmd = &cobra.Command{
	Use:   "skus",
	Short: "provides skus service support tooling",
}

var resetLinkingLimitCmd = &cobra.Command{
	Use:   "reset-linking-limit",
	Short: "Free device linking slots for a premium order",
	Long: `Frees N device linking slots for a TLV2 order by deleting the oldest active
credential batches. Each batch corresponds to one linked device.

The order can be identified by --order-id or by --email (which looks up
the subscriber in the subscriptions service). If multiple orders match an
email you will be prompted to choose one.

The command shows which batches will be removed and asks for confirmation
before making any changes.

Note: email lookup only works for desktop/browser orders created through
the subscriptions service. iOS and Android orders use anonymous receipts
and cannot be looked up by email.`,
	RunE: runResetLinkingLimit,
}

func init() {
	SkusCmd.AddCommand(resetLinkingLimitCmd)
	rootcmd.RootCmd.AddCommand(SkusCmd)

	fb := rootcmd.NewFlagBuilder(resetLinkingLimitCmd)

	fb.Flag().String("skus-base-url", "",
		"base URL of the SKUs service (e.g. https://payment.rewards.brave.com)").
		Env("SKUS_BASE_URL").
		Bind("skus-base-url")

	fb.Flag().String("order-id", "",
		"the order UUID to reset linking slots for (mutually exclusive with --email)").
		Bind("order-id")

	fb.Flag().String("email", "",
		"subscriber email to look up the order ID (mutually exclusive with --order-id)").
		Env("SUBSCRIBER_EMAIL").
		Bind("email")

	fb.Flag().String("subscriptions-base-url", "",
		"base URL of the subscriptions service, required when using --email").
		Env("SUBSCRIPTIONS_BASE_URL").
		Bind("subscriptions-base-url")

	fb.Flag().String("subscriptions-token", "",
		"bearer token for the subscriptions support API, required when using --email").
		Env("SUBSCRIPTIONS_SUPPORT_TOKEN").
		Bind("subscriptions-token")

	fb.Flag().Int("seats", 0,
		"number of device slots to free (deletes this many oldest batches)").
		Bind("seats").
		Require()

	fb.Flag().String("item-id", "",
		"optional: scope the reset to a specific order item UUID").
		Bind("item-id")

	fb.Flag().String("private-key", "",
		"path to the ed25519 private key file in SSH format used to sign requests").
		Env("SKUS_SUPPORT_PRIVATE_KEY").
		Bind("private-key")
}

func runResetLinkingLimit(cmd *cobra.Command, args []string) error {
	baseURL := strings.TrimRight(viper.GetString("skus-base-url"), "/")
	orderID := viper.GetString("order-id")
	email := strings.TrimSpace(viper.GetString("email"))
	seats := viper.GetInt("seats")
	itemID := viper.GetString("item-id")

	if baseURL == "" {
		return fmt.Errorf("--skus-base-url (or SKUS_BASE_URL) is required")
	}

	if viper.GetString("private-key") == "" {
		return fmt.Errorf("--private-key (or SKUS_SUPPORT_PRIVATE_KEY) is required")
	}

	if seats <= 0 {
		return fmt.Errorf("--seats must be a positive integer")
	}

	switch {
	case orderID == "" && email == "":
		return fmt.Errorf("one of --order-id or --email is required")
	case orderID != "" && email != "":
		return fmt.Errorf("--order-id and --email are mutually exclusive")
	}

	privKey, err := loadED25519PrivateKey(viper.GetString("private-key"))
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	if email != "" {
		subsBaseURL := strings.TrimRight(viper.GetString("subscriptions-base-url"), "/")
		subsToken := viper.GetString("subscriptions-token")

		if subsBaseURL == "" {
			return fmt.Errorf("--subscriptions-base-url is required when using --email")
		}

		if subsToken == "" {
			return fmt.Errorf("--subscriptions-token is required when using --email")
		}

		orderID, err = resolveOrderIDByEmail(ctx, client, subsBaseURL, email, subsToken)
		if err != nil {
			return err
		}
	}

	listURL := fmt.Sprintf("%s/v1/orders/%s/credentials/batches", baseURL, orderID)
	if itemID != "" {
		listURL += "?" + url.Values{"item_id": {itemID}}.Encode()
	}

	batches, err := listBatches(ctx, client, listURL, privKey)
	if err != nil {
		return fmt.Errorf("failed to list batches: %w", err)
	}

	if len(batches) == 0 {
		fmt.Println("No active device batches found for this order.")
		return nil
	}

	fmt.Printf("Order %s has %d active device batch(es).\n\n", orderID, len(batches))

	if seats > len(batches) {
		fmt.Printf("Warning: --seats (%d) exceeds active batch count (%d). All batches will be removed.\n\n", seats, len(batches))
		seats = len(batches)
	}

	fmt.Printf("Oldest %d batch(es) at time of listing:\n\n", seats)
	fmt.Printf("  %-40s  %s\n", "request_id", "oldest_valid_from (UTC)")
	fmt.Printf("  %-40s  %s\n", strings.Repeat("-", 40), strings.Repeat("-", 24))
	for _, b := range batches[:seats] {
		fmt.Printf("  %-40s  %s\n", b.RequestID, b.OldestValidFrom.UTC().Format(time.RFC3339))
	}
	fmt.Println()
	fmt.Println("Note: the server selects the oldest N batches independently at delete time.")
	fmt.Println("      If the order changes before the request arrives, the result may differ.")
	fmt.Println()

	if !confirm(fmt.Sprintf("Delete %d seat(s) for order %s?", seats, orderID)) {
		fmt.Println("Aborted.")
		return nil
	}

	deleteURL := fmt.Sprintf("%s/v1/orders/%s/credentials/batches", baseURL, orderID)
	if err := deleteBatchSeats(ctx, client, deleteURL, privKey, seats, itemID); err != nil {
		return fmt.Errorf("failed to delete batch seats: %w", err)
	}

	fmt.Printf("Done. %d device slot(s) freed for order %s.\n", seats, orderID)

	return nil
}

type activeSubsResp struct {
	Email       string `json:"email"`
	OrderID     string `json:"order_id"`
	ProductName string `json:"product_name"`
}

type activeSubsListResp struct {
	Results []activeSubsResp `json:"results"`
}

func resolveOrderIDByEmail(ctx context.Context, client *http.Client, baseURL, email, token string) (string, error) {
	u := baseURL + "/v1/support/subscribers/" + url.PathEscape(email)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no subscriber found for email %q", email)
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return "", fmt.Errorf("unexpected status %d: failed to read response body: %w", resp.StatusCode, err)
		}
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result activeSubsListResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Results) == 0 {
		return "", fmt.Errorf("no active subscriptions found for email %q", email)
	}

	if len(result.Results) == 1 {
		r := result.Results[0]
		fmt.Printf("Found 1 active subscription for %s (%s)\n\n", r.Email, r.ProductName)
		return r.OrderID, nil
	}

	fmt.Printf("Found %d active subscriptions matching %q:\n\n", len(result.Results), email)
	fmt.Printf("  %-3s  %-36s  %-20s  %s\n", "#", "order_id", "product", "email")
	fmt.Printf("  %-3s  %-36s  %-20s  %s\n",
		"---", strings.Repeat("-", 36), strings.Repeat("-", 20), strings.Repeat("-", 30))

	for i, r := range result.Results {
		fmt.Printf("  %-3d  %-36s  %-20s  %s\n", i+1, r.OrderID, r.ProductName, r.Email)
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("Select subscription [1-%d]: ", len(result.Results))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("reading stdin: %w", err)
			}
			return "", fmt.Errorf("no selection made")
		}

		n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil || n < 1 || n > len(result.Results) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(result.Results))
			continue
		}

		return result.Results[n-1].OrderID, nil
	}
}

func listBatches(ctx context.Context, client *http.Client, endpoint string, key ed25519.PrivateKey) ([]model.TLV2ActiveBatch, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	if err := skus.SignSupportRequest(key, req); err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Batches []model.TLV2ActiveBatch `json:"batches"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Batches, nil
}

func deleteBatchSeats(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	key ed25519.PrivateKey,
	seats int,
	itemID string,
) error {
	payload := struct {
		Seats  int    `json:"seats"`
		ItemID string `json:"item_id,omitempty"`
	}{Seats: seats, ItemID: itemID}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if err := skus.SignSupportRequest(key, req); err != nil {
		return fmt.Errorf("failed to sign request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

func loadED25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	raw, err := ssh.ParseRawPrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH private key: %w", err)
	}

	key, ok := raw.(*ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not an ed25519 private key (got %T)", raw)
	}

	return *key, nil
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
}
