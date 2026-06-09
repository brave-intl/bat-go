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

var showLinkingUsageCmd = &cobra.Command{
	Use:   "show-linking-usage",
	Short: "Show device linking slot usage for a premium order",
	Long: `Shows how many device linking slots are in use for a TLV2 order by listing
its active credential batches. Each batch corresponds to one linked device.

This is the read-only counterpart of reset-linking-limit: it lists the same
batches the deletion flow shows, but never makes changes.

The order can be identified by --order-id or by --email (which looks up
the subscriber in the subscriptions service). If multiple orders match an
email you will be prompted to choose one.

Note: email lookup requires the order to be linked to a Premium account.
Mobile (iOS/Android) purchases are found by email once linked; an order that
has only been created anonymously and not yet linked has no associated email.`,
	RunE: runShowLinkingUsage,
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

	SkusCmd.AddCommand(showLinkingUsageCmd)

	// These flags are intentionally not bound to viper. reset-linking-limit
	// already binds the global viper keys for these names to its own flags,
	// and viper.BindPFlag is global, so binding them again here would clobber
	// that command. runShowLinkingUsage reads these directly from the command,
	// falling back to the environment.
	sfb := rootcmd.NewFlagBuilder(showLinkingUsageCmd)

	sfb.Flag().String("skus-base-url", "",
		"base URL of the SKUs service, e.g. https://payment.rewards.brave.com [env: SKUS_BASE_URL]")

	sfb.Flag().String("order-id", "",
		"the order UUID to show slot usage for (mutually exclusive with --email)")

	sfb.Flag().String("email", "",
		"subscriber email to look up the order ID (mutually exclusive with --order-id) [env: SUBSCRIBER_EMAIL]")

	sfb.Flag().String("subscriptions-base-url", "",
		"base URL of the subscriptions service, required when using --email [env: SUBSCRIPTIONS_BASE_URL]")

	sfb.Flag().String("subscriptions-token", "",
		"bearer token for the subscriptions support API, required when using --email [env: SUBSCRIPTIONS_SUPPORT_TOKEN]")

	sfb.Flag().String("item-id", "",
		"optional: scope the listing to a specific order item UUID")

	sfb.Flag().String("private-key", "",
		"path to the ed25519 private key file in SSH format used to sign requests [env: SKUS_SUPPORT_PRIVATE_KEY]")
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

	if err := requireOrderRef(orderID, email); err != nil {
		return err
	}

	privKey, err := loadED25519PrivateKey(viper.GetString("private-key"))
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	subsBaseURL := strings.TrimRight(viper.GetString("subscriptions-base-url"), "/")
	subsToken := viper.GetString("subscriptions-token")

	orderID, err = resolveOrderID(ctx, client, orderID, email, subsBaseURL, subsToken)
	if err != nil {
		return err
	}

	batches, err := listBatches(ctx, client, batchesURL(baseURL, orderID, itemID), privKey)
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
	fmt.Print(formatBatchTable(batches[:seats]))
	fmt.Println()
	fmt.Println("Note: the server selects the oldest N batches independently at delete time.")
	fmt.Println("      If the order changes before the request arrives, the result may differ.")
	fmt.Println()

	if !confirm(fmt.Sprintf("Delete %d seat(s) for order %s?", seats, orderID)) {
		fmt.Println("Aborted.")
		return nil
	}

	if err := deleteBatchSeats(ctx, client, batchesURL(baseURL, orderID, ""), privKey, seats, itemID); err != nil {
		return fmt.Errorf("failed to delete batch seats: %w", err)
	}

	fmt.Printf("Done. %d device slot(s) freed for order %s.\n", seats, orderID)

	return nil
}

// flagOrEnv returns the command-line flag value, falling back to the environment
// variable when the flag is unset. show-linking-usage reads its flags this way
// rather than through the shared viper bindings used by reset-linking-limit.
func flagOrEnv(cmd *cobra.Command, flag, env string) string {
	if v, _ := cmd.Flags().GetString(flag); v != "" {
		return v
	}

	return os.Getenv(env)
}

func runShowLinkingUsage(cmd *cobra.Command, args []string) error {
	baseURL := strings.TrimRight(flagOrEnv(cmd, "skus-base-url", "SKUS_BASE_URL"), "/")
	email := strings.TrimSpace(flagOrEnv(cmd, "email", "SUBSCRIBER_EMAIL"))
	privKeyPath := flagOrEnv(cmd, "private-key", "SKUS_SUPPORT_PRIVATE_KEY")

	orderID, _ := cmd.Flags().GetString("order-id")
	orderID = strings.TrimSpace(orderID)

	itemID, _ := cmd.Flags().GetString("item-id")
	itemID = strings.TrimSpace(itemID)

	if baseURL == "" {
		return fmt.Errorf("--skus-base-url (or SKUS_BASE_URL) is required")
	}

	if privKeyPath == "" {
		return fmt.Errorf("--private-key (or SKUS_SUPPORT_PRIVATE_KEY) is required")
	}

	if err := requireOrderRef(orderID, email); err != nil {
		return err
	}

	privKey, err := loadED25519PrivateKey(privKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	subsBaseURL := strings.TrimRight(flagOrEnv(cmd, "subscriptions-base-url", "SUBSCRIPTIONS_BASE_URL"), "/")
	subsToken := flagOrEnv(cmd, "subscriptions-token", "SUBSCRIPTIONS_SUPPORT_TOKEN")

	orderID, err = resolveOrderID(ctx, client, orderID, email, subsBaseURL, subsToken)
	if err != nil {
		return err
	}

	batches, err := listBatches(ctx, client, batchesURL(baseURL, orderID, itemID), privKey)
	if err != nil {
		return fmt.Errorf("failed to list batches: %w", err)
	}

	fmt.Print(formatLinkingUsage(orderID, itemID, batches))

	return nil
}

// requireOrderRef validates that exactly one of --order-id and --email was given.
func requireOrderRef(orderID, email string) error {
	switch {
	case orderID == "" && email == "":
		return fmt.Errorf("one of --order-id or --email is required")
	case orderID != "" && email != "":
		return fmt.Errorf("--order-id and --email are mutually exclusive")
	}

	return nil
}

// resolveOrderID returns orderID as-is when set, otherwise resolves email to an
// order ID via the subscriptions support API, validating the flags it needs.
func resolveOrderID(ctx context.Context, client *http.Client, orderID, email, subsBaseURL, subsToken string) (string, error) {
	if orderID != "" {
		return orderID, nil
	}

	if subsBaseURL == "" {
		return "", fmt.Errorf("--subscriptions-base-url is required when using --email")
	}

	if subsToken == "" {
		return "", fmt.Errorf("--subscriptions-token is required when using --email")
	}

	return resolveOrderIDByEmail(ctx, client, subsBaseURL, email, subsToken)
}

// batchesURL builds the SKUs credential batches endpoint URL for an order,
// scoped to an order item when itemID is non-empty.
func batchesURL(baseURL, orderID, itemID string) string {
	u := fmt.Sprintf("%s/v1/orders/%s/credentials/batches", baseURL, orderID)
	if itemID != "" {
		u += "?" + url.Values{"item_id": {itemID}}.Encode()
	}

	return u
}

// formatLinkingUsage renders the slot usage report for an order. Each active
// batch corresponds to one linked device, so the batch count is the number of
// device slots in use.
func formatLinkingUsage(orderID, itemID string, batches []model.TLV2ActiveBatch) string {
	var b strings.Builder

	scope := fmt.Sprintf("order %s", orderID)
	if itemID != "" {
		scope = fmt.Sprintf("order %s (item %s)", orderID, itemID)
	}

	if len(batches) == 0 {
		fmt.Fprintf(&b, "No device slots are in use for %s.\n", scope)
		return b.String()
	}

	fmt.Fprintf(&b, "%d device slot(s) in use for %s.\n\n", len(batches), scope)
	b.WriteString(formatBatchTable(batches))

	return b.String()
}

// formatBatchTable renders the request_id / oldest_valid_from table shared by
// the usage report and the reset confirmation listing.
func formatBatchTable(batches []model.TLV2ActiveBatch) string {
	var b strings.Builder

	fmt.Fprintf(&b, "  %-40s  %s\n", "request_id", "oldest_valid_from (UTC)")
	fmt.Fprintf(&b, "  %-40s  %s\n", strings.Repeat("-", 40), strings.Repeat("-", 24))
	for _, batch := range batches {
		fmt.Fprintf(&b, "  %-40s  %s\n", batch.RequestID, batch.OldestValidFrom.UTC().Format(time.RFC3339))
	}

	return b.String()
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
