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
		Bind("skus-base-url").
		Require()

	fb.Flag().String("order-id", "",
		"the order UUID to reset linking slots for (mutually exclusive with --email)").
		Bind("order-id")

	fb.Flag().String("email", "",
		"subscriber email to look up orders for (mutually exclusive with --order-id)").
		Bind("email")

	fb.Flag().String("subscriptions-base-url", "",
		"base URL of the subscriptions service, required when --email is used").
		Env("SUBSCRIPTIONS_BASE_URL").
		Bind("subscriptions-base-url")

	fb.Flag().String("subscriptions-token", "",
		"bearer token for the subscriptions service support API, required when --email is used").
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
		Bind("private-key").
		Require()
}

func runResetLinkingLimit(cmd *cobra.Command, args []string) error {
	baseURL := strings.TrimRight(viper.GetString("skus-base-url"), "/")
	orderID := viper.GetString("order-id")
	email := strings.TrimSpace(viper.GetString("email"))
	seats := viper.GetInt("seats")
	itemID := viper.GetString("item-id")

	if seats <= 0 {
		return fmt.Errorf("--seats must be a positive integer")
	}

	if orderID == "" && email == "" {
		return fmt.Errorf("one of --order-id or --email is required")
	}

	if orderID != "" && email != "" {
		return fmt.Errorf("--order-id and --email are mutually exclusive")
	}

	privKey, err := loadED25519PrivateKey(viper.GetString("private-key"))
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	if email != "" {
		subsURL := strings.TrimRight(viper.GetString("subscriptions-base-url"), "/")
		subsToken := viper.GetString("subscriptions-token")

		if subsURL == "" {
			return fmt.Errorf("--subscriptions-base-url (or SUBSCRIPTIONS_BASE_URL) is required when --email is used")
		}
		if subsToken == "" {
			return fmt.Errorf("--subscriptions-token (or SUBSCRIPTIONS_SUPPORT_TOKEN) is required when --email is used")
		}

		resolved, err := resolveOrderIDByEmail(ctx, client, subsURL, subsToken, email)
		if err != nil {
			return fmt.Errorf("email lookup failed: %w", err)
		}

		orderID = resolved
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

// subsSearchResult mirrors the JSON returned by the subscriptions service support endpoint.
type subsSearchResult struct {
	SubscriberID   string     `json:"subscriber_id"`
	Email          string     `json:"email"`
	SubscriptionID string     `json:"subscription_id"`
	OrderID        string     `json:"order_id"`
	ProductName    string     `json:"product_name"`
	CreatedAt      *time.Time `json:"created_at"`
}

// resolveOrderIDByEmail queries the subscriptions service for orders associated with
// the given email fragment, presents the results to the support operator, and returns
// the chosen order ID.
func resolveOrderIDByEmail(
	ctx context.Context,
	client *http.Client,
	subsURL,
	token, email string,
) (string, error) {
	if len(email) < 6 {
		return "", fmt.Errorf("email must be at least 6 characters")
	}

	searchURL := subsURL + "/v1/support/subscribers/search?" + url.Values{"email": {email}}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("subscriptions service returned %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Results []subsSearchResult `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	matches := result.Results
	if len(matches) == 0 {
		return "", fmt.Errorf("no orders found for email %q", email)
	}

	if len(matches) == 1 {
		fmt.Printf("Found 1 order for %s (subscriber %s)\n\n", matches[0].Email, matches[0].SubscriberID)
		return matches[0].OrderID, nil
	}

	fmt.Printf("Found %d order(s) matching %q:\n\n", len(matches), email)
	fmt.Printf("  %-3s  %-36s  %-20s  %-40s  %-20s  %s\n",
		"#", "order_id", "product", "subscription_id", "created_at", "email")
	fmt.Printf("  %-3s  %-36s  %-20s  %-40s  %-20s  %s\n",
		"---", strings.Repeat("-", 36), strings.Repeat("-", 20), strings.Repeat("-", 40), strings.Repeat("-", 20), strings.Repeat("-", 30))

	for i, r := range matches {
		created := ""
		if r.CreatedAt != nil {
			created = r.CreatedAt.UTC().Format("2006-01-02 15:04")
		}
		fmt.Printf("  %-3d  %-36s  %-20s  %-40s  %-20s  %s\n",
			i+1, r.OrderID, r.ProductName, r.SubscriptionID, created, r.Email)
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("Select order [1-%d]: ", len(matches))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("reading stdin: %w", err)
			}
			return "", fmt.Errorf("no selection made")
		}

		n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil || n < 1 || n > len(matches) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(matches))
			continue
		}

		return matches[n-1].OrderID, nil
	}
}

func listBatches(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	key ed25519.PrivateKey,
) ([]model.TLV2ActiveBatch, error) {
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
