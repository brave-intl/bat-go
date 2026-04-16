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

The command shows which batches will be removed and asks for confirmation
before making any changes.`,
	RunE: runResetLinkingLimit,
}

var relinkSubscriptionCmd = &cobra.Command{
	Use:   "relink-subscription",
	Short: "Relink a Premium order to a new active Stripe subscription",
	Long: `Points a Premium order at a new Stripe subscription and renews it.

Use this when a user has canceled and resubscribed but their account still
references the old canceled subscription, preventing new credentials from loading.`,
	RunE: runRelinkSubscription,
}

func init() {
	SkusCmd.AddCommand(resetLinkingLimitCmd)
	SkusCmd.AddCommand(relinkSubscriptionCmd)
	rootcmd.RootCmd.AddCommand(SkusCmd)

	{
		rl := relinkSubscriptionCmd.Flags()
		rl.String("skus-base-url", "", "base URL of the SKUs service (e.g. https://payment.rewards.brave.com)")
		rl.String("order-id", "", "the order UUID to relink")
		rl.String("subscription-id", "", "the new active Stripe subscription ID (e.g. sub_...)")
		rl.String("private-key", "", "path to the ed25519 private key file in SSH format used to sign requests")
		rootcmd.Must(relinkSubscriptionCmd.MarkFlagRequired("skus-base-url"))
		rootcmd.Must(relinkSubscriptionCmd.MarkFlagRequired("order-id"))
		rootcmd.Must(relinkSubscriptionCmd.MarkFlagRequired("subscription-id"))
		rootcmd.Must(relinkSubscriptionCmd.MarkFlagRequired("private-key"))
	}

	fb := rootcmd.NewFlagBuilder(resetLinkingLimitCmd)

	fb.Flag().String("skus-base-url", "",
		"base URL of the SKUs service (e.g. https://payment.rewards.brave.com)").
		Env("SKUS_BASE_URL").
		Bind("skus-base-url").
		Require()

	fb.Flag().String("order-id", "",
		"the order UUID to reset linking slots for").
		Bind("order-id").
		Require()

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
	seats := viper.GetInt("seats")
	itemID := viper.GetString("item-id")

	if seats <= 0 {
		return fmt.Errorf("--seats must be a positive integer")
	}

	privKey, err := loadED25519PrivateKey(viper.GetString("private-key"))
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

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

func deleteBatchSeats(ctx context.Context, client *http.Client, endpoint string, key ed25519.PrivateKey, seats int, itemID string) error {
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

func runRelinkSubscription(cmd *cobra.Command, args []string) error {
	baseURL, _ := cmd.Flags().GetString("skus-base-url")
	baseURL = strings.TrimRight(baseURL, "/")
	orderID, _ := cmd.Flags().GetString("order-id")
	subID, _ := cmd.Flags().GetString("subscription-id")
	privKeyPath, _ := cmd.Flags().GetString("private-key")

	privKey, err := loadED25519PrivateKey(privKeyPath)
	if err != nil {
		return fmt.Errorf("failed to load private key: %w", err)
	}

	if !confirm(fmt.Sprintf("Relink order %s to subscription %s?", orderID, subID)) {
		fmt.Println("Aborted.")
		return nil
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	endpoint := fmt.Sprintf("%s/v1/orders/%s/subscription", baseURL, orderID)

	payload := struct {
		SubscriptionID string `json:"subscriptionId"`
	}{SubscriptionID: subID}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	if err := skus.SignSupportRequest(privKey, req); err != nil {
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

	fmt.Printf("Done. Order %s relinked to subscription %s.\n", orderID, subID)

	return nil
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
}
