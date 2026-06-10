// Package cmd implements the skus support CLI.
//
// All commands talk exclusively to the subscriptions service support API with
// a bearer token.
//
// The registration onto rootcmd.RootCmd in init is the only coupling to the
// bat-go repo; everything else is stdlib plus cobra, so the package can be
// lifted into its own repository.
package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	rootcmd "github.com/brave-intl/bat-go/cmd"
)

var SkusCmd = &cobra.Command{
	Use:   "skus",
	Short: "provides skus service support tooling",
}

var showLinkingUsageCmd = &cobra.Command{
	Use:   "show-linking-usage",
	Short: "Show device linking slot usage for a premium subscription",
	Long: `Shows how many device linking slots are in use for a subscription, out of its
current limit, with one row per linked device. Read-only; never makes changes.

The subscriber is identified by --email (looked up via the subscriptions
support API). If multiple subscriptions match you will be prompted to choose
one. Alternatively pass --subscription-id to target a subscription directly.

Note: email lookup requires the subscription to be linked to a Premium account.
Mobile (iOS/Android) purchases are found by email once linked; an order that
has only been created anonymously and not yet linked has no associated email.`,
	RunE: runShowLinkingUsage,
}

var resetLinkingLimitCmd = &cobra.Command{
	Use:   "reset-linking-limit",
	Short: "Free device linking slots for a premium subscription",
	Long: `Frees N device linking slots for a subscription by deleting the oldest active
credential batches. Each batch corresponds to one linked device.

The command goes through the subscriptions service support API, which owns the
operation and rejects a reset that exceeds the slots currently in use.

The subscriber is identified by --email (looked up via the subscriptions
support API). If multiple subscriptions match you will be prompted to choose
one. Alternatively pass --subscription-id to target a subscription directly.

The command shows the current slot usage and asks for confirmation before
making any changes.

Note: email lookup requires the subscription to be linked to a Premium account.
Mobile (iOS/Android) purchases are found by email once linked; an order that
has only been created anonymously and not yet linked has no associated email.`,
	RunE: runResetLinkingLimit,
}

var extendLinkingLimitCmd = &cobra.Command{
	Use:   "extend-linking-limit",
	Short: "Grant a device-linking-limit extension for a premium subscriber",
	Long: `Grants a self-service-style linking-limit extension for a TLV2 subscriber,
adding device slots without first unlinking a device.

Subscriptions owns and enforces the same policy as the in-browser self-service
flow, so the same gates apply:

  - the subscriber must currently be at their device limit,
  - extensions are capped per subscription over its lifetime,
  - and are rate-limited (one per window).

The subscriber is identified by --email (looked up via the subscriptions
support API). If multiple subscriptions match you will be prompted to choose
one. Alternatively pass --subscription-id to target a subscription directly.

If the subscriber is not eligible (not at limit, rate-limited, or cap reached)
the command reports the reason and makes no change.

Note: email lookup requires the subscription to be linked to a Premium account.
Mobile (iOS/Android) purchases are found by email once linked; an order that has
only been created anonymously and not yet linked has no associated email.`,
	RunE: runExtendLinkingLimit,
}

func init() {
	rootcmd.RootCmd.AddCommand(SkusCmd)

	SkusCmd.AddCommand(showLinkingUsageCmd)
	SkusCmd.AddCommand(resetLinkingLimitCmd)
	SkusCmd.AddCommand(extendLinkingLimitCmd)

	addSupportFlags(showLinkingUsageCmd)
	addSupportFlags(resetLinkingLimitCmd)
	addSupportFlags(extendLinkingLimitCmd)

	resetLinkingLimitCmd.Flags().Int("seats", 0,
		"number of device slots to free (deletes this many oldest batches)")
}

// addSupportFlags registers the flags shared by every support command.
func addSupportFlags(cmd *cobra.Command) {
	cmd.Flags().String("subscriptions-base-url", "",
		"base URL of the subscriptions service, e.g. https://subscriptions.brave.com [env: SUBSCRIPTIONS_BASE_URL]")

	cmd.Flags().String("subscriptions-token", "",
		"bearer token for the subscriptions support API [env: SUBSCRIPTIONS_SUPPORT_TOKEN]")

	cmd.Flags().String("email", "",
		"subscriber email to look up the subscription (mutually exclusive with --subscription-id) [env: SUBSCRIBER_EMAIL]")

	cmd.Flags().String("subscription-id", "",
		"the subscription UUID to target (mutually exclusive with --email)")
}

// flagOrEnv returns the command-line flag value, falling back to the environment
// variable when the flag is unset.
func flagOrEnv(cmd *cobra.Command, flag, env string) string {
	if v, _ := cmd.Flags().GetString(flag); v != "" {
		return v
	}

	return os.Getenv(env)
}

// subsConn reads and validates the subscriptions support API connection flags,
// returning the base URL and bearer token.
func subsConn(cmd *cobra.Command) (string, string, error) {
	baseURL := strings.TrimRight(flagOrEnv(cmd, "subscriptions-base-url", "SUBSCRIPTIONS_BASE_URL"), "/")
	token := flagOrEnv(cmd, "subscriptions-token", "SUBSCRIPTIONS_SUPPORT_TOKEN")

	if baseURL == "" {
		return "", "", fmt.Errorf("--subscriptions-base-url (or SUBSCRIPTIONS_BASE_URL) is required")
	}

	if token == "" {
		return "", "", fmt.Errorf("--subscriptions-token (or SUBSCRIPTIONS_SUPPORT_TOKEN) is required")
	}

	return baseURL, token, nil
}

// subRef reads the subscription identification flags.
func subRef(cmd *cobra.Command) (string, string) {
	subID, _ := cmd.Flags().GetString("subscription-id")
	email := flagOrEnv(cmd, "email", "SUBSCRIBER_EMAIL")

	return strings.TrimSpace(subID), strings.TrimSpace(email)
}

// requireSubRef validates that exactly one of --subscription-id and --email
// was given.
func requireSubRef(subID, email string) error {
	switch {
	case subID == "" && email == "":
		return fmt.Errorf("one of --email or --subscription-id is required")
	case subID != "" && email != "":
		return fmt.Errorf("--email and --subscription-id are mutually exclusive")
	}

	return nil
}

// resolveSubID returns the target subscription ID, resolving --email via the
// support API when --subscription-id was not given. The second return value is
// the product name when known (email path), for display.
func resolveSubID(ctx context.Context, client *http.Client, baseURL, token, subID, email string) (string, string, error) {
	if subID != "" {
		return subID, "", nil
	}

	sub, err := resolveSubByEmail(ctx, client, baseURL, email, token)
	if err != nil {
		return "", "", err
	}

	if sub.SubscriptionID == "" {
		return "", "", fmt.Errorf("subscription for %q has no subscription_id", email)
	}

	return sub.SubscriptionID, sub.ProductName, nil
}

func runShowLinkingUsage(cmd *cobra.Command, args []string) error {
	baseURL, token, err := subsConn(cmd)
	if err != nil {
		return err
	}

	subID, email := subRef(cmd)
	if err := requireSubRef(subID, email); err != nil {
		return err
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	subID, productName, err := resolveSubID(ctx, client, baseURL, token, subID, email)
	if err != nil {
		return err
	}

	usage, err := getLinkingUsage(ctx, client, baseURL, subID, token)
	if err != nil {
		return err
	}

	fmt.Print(formatLinkingUsage(subID, productName, usage))

	return nil
}

func runResetLinkingLimit(cmd *cobra.Command, args []string) error {
	baseURL, token, err := subsConn(cmd)
	if err != nil {
		return err
	}

	seats, _ := cmd.Flags().GetInt("seats")
	if seats <= 0 {
		return fmt.Errorf("--seats must be a positive integer")
	}

	subID, email := subRef(cmd)
	if err := requireSubRef(subID, email); err != nil {
		return err
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	subID, productName, err := resolveSubID(ctx, client, baseURL, token, subID, email)
	if err != nil {
		return err
	}

	usage, err := getLinkingUsage(ctx, client, baseURL, subID, token)
	if err != nil {
		return err
	}

	fmt.Print(formatLinkingUsage(subID, productName, usage))
	fmt.Println()

	if seats > usage.Active {
		return fmt.Errorf("--seats (%d) exceeds device slots in use (%d)", seats, usage.Active)
	}

	fmt.Println("Note: the server selects the oldest N batches independently at delete time.")
	fmt.Println("      If the subscription changes before the request arrives, the result may differ.")
	fmt.Println()

	if !confirm(fmt.Sprintf("Delete %d seat(s) for subscription %s?", seats, subID)) {
		fmt.Println("Aborted.")
		return nil
	}

	if err := resetLinkingSlots(ctx, client, baseURL, subID, token, seats); err != nil {
		return err
	}

	fmt.Printf("Done. %d device slot(s) freed for subscription %s.\n", seats, subID)

	return nil
}

func runExtendLinkingLimit(cmd *cobra.Command, args []string) error {
	baseURL, token, err := subsConn(cmd)
	if err != nil {
		return err
	}

	subID, email := subRef(cmd)
	if err := requireSubRef(subID, email); err != nil {
		return err
	}

	ctx := cmd.Context()
	client := &http.Client{Timeout: 30 * time.Second}

	subID, productName, err := resolveSubID(ctx, client, baseURL, token, subID, email)
	if err != nil {
		return err
	}

	prompt := fmt.Sprintf("Extend the device linking limit for subscription %s?", subID)
	if productName != "" {
		prompt = fmt.Sprintf("Extend the device linking limit for subscription %s (%s)?", subID, productName)
	}

	fmt.Println("Subscriptions enforces the self-service policy: the subscriber must be at")
	fmt.Println("their device limit, under the lifetime extension cap, and outside the rate")
	fmt.Println("limit window. If they are not eligible, no change is made.")
	fmt.Println()

	if !confirm(prompt) {
		fmt.Println("Aborted.")
		return nil
	}

	if err := extendLinkingLimit(ctx, client, baseURL, subID, token); err != nil {
		return err
	}

	fmt.Printf("Done. Linking limit extended for subscription %s.\n", subID)

	return nil
}

// linkingBatch is one active credential batch, corresponding to one linked device.
type linkingBatch struct {
	RequestID       string    `json:"request_id"`
	OldestValidFrom time.Time `json:"oldest_valid_from"`
}

// linkingUsageResp is the support API's slot usage report for a subscription.
type linkingUsageResp struct {
	Limit   int            `json:"limit"`
	Active  int            `json:"active"`
	Batches []linkingBatch `json:"batches"`
}

// respBodyLimit caps reads of success response bodies; errBodyLimit caps
// reads of error response bodies, which only feed error messages.
const (
	respBodyLimit = 1 << 20
	errBodyLimit  = 4096
)

// supportBatchesURL builds the support API credential batches endpoint URL
// for a subscription.
func supportBatchesURL(baseURL, subID string) string {
	return fmt.Sprintf("%s/v1/support/subscriptions/%s/credentials/batches", baseURL, url.PathEscape(subID))
}

// appError mirrors the subscriptions support API error envelope.
type appError struct {
	Message   string `json:"message"`
	ErrorCode string `json:"errorCode"`
}

// supportAPIError renders a non-200 support API response body as an error,
// surfacing the envelope's errorCode and message when present.
func supportAPIError(action string, statusCode int, body []byte) error {
	var ae appError
	if err := json.Unmarshal(body, &ae); err != nil || ae.Message == "" {
		return fmt.Errorf("%s failed: unexpected status %d: %s", action, statusCode, body)
	}

	if ae.ErrorCode != "" {
		return fmt.Errorf("%s failed (status %d, %s): %s", action, statusCode, ae.ErrorCode, ae.Message)
	}

	return fmt.Errorf("%s failed (status %d): %s", action, statusCode, ae.Message)
}

// getLinkingUsage fetches the slot usage report for a subscription.
func getLinkingUsage(ctx context.Context, client *http.Client, baseURL, subID, token string) (*linkingUsageResp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, supportBatchesURL(baseURL, subID), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, respBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("usage lookup failed: status %d: failed to read response body: %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, supportAPIError("usage lookup", resp.StatusCode, body)
	}

	result := &linkingUsageResp{}
	if err := json.Unmarshal(body, result); err != nil {
		return nil, fmt.Errorf("failed to decode usage response: %w", err)
	}

	return result, nil
}

// resetLinkingSlots frees device slots for a subscription by deleting its
// oldest credential batches.
func resetLinkingSlots(ctx context.Context, client *http.Client, baseURL, subID, token string, seats int) error {
	endpoint := supportBatchesURL(baseURL, subID)

	payload, err := json.Marshal(struct {
		Seats int `json:"seats"`
	}{Seats: seats})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
	if err != nil {
		return fmt.Errorf("reset failed: unexpected status %d: failed to read response body: %w", resp.StatusCode, err)
	}

	return supportAPIError("reset", resp.StatusCode, body)
}

// extendLinkingLimit calls the subscriptions support API to grant an extension.
func extendLinkingLimit(ctx context.Context, client *http.Client, baseURL, subID, token string) error {
	endpoint := supportBatchesURL(baseURL, subID) + "/extend"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, errBodyLimit))
	if err != nil {
		return fmt.Errorf("extension failed: unexpected status %d: failed to read response body: %w", resp.StatusCode, err)
	}

	return supportAPIError("extension", resp.StatusCode, body)
}

// formatLinkingUsage renders the slot usage report for a subscription, with
// one row per linked device.
func formatLinkingUsage(subID, productName string, usage *linkingUsageResp) string {
	var b strings.Builder

	scope := fmt.Sprintf("subscription %s", subID)
	if productName != "" {
		scope = fmt.Sprintf("subscription %s (%s)", subID, productName)
	}

	fmt.Fprintf(&b, "%d of %d device slot(s) in use for %s.\n", usage.Active, usage.Limit, scope)

	if len(usage.Batches) == 0 {
		return b.String()
	}

	b.WriteString("\n")
	b.WriteString(formatBatchTable(usage.Batches))

	return b.String()
}

// formatBatchTable renders active batches as a request_id / oldest_valid_from table.
func formatBatchTable(batches []linkingBatch) string {
	var b strings.Builder

	fmt.Fprintf(&b, "  %-40s  %s\n", "request_id", "oldest_valid_from (UTC)")
	fmt.Fprintf(&b, "  %-40s  %s\n", strings.Repeat("-", 40), strings.Repeat("-", 24))
	for _, batch := range batches {
		fmt.Fprintf(&b, "  %-40s  %s\n", batch.RequestID, batch.OldestValidFrom.UTC().Format(time.RFC3339))
	}

	return b.String()
}

type activeSubsResp struct {
	Email          string `json:"email"`
	SubscriptionID string `json:"subscription_id"`
	OrderID        string `json:"order_id"`
	ProductName    string `json:"product_name"`
}

type activeSubsListResp struct {
	Results []activeSubsResp `json:"results"`
}

// resolveSubByEmail fetches the active subscriptions for an email and returns the
// chosen one, prompting the operator when more than one matches.
func resolveSubByEmail(ctx context.Context, client *http.Client, baseURL, email, token string) (activeSubsResp, error) {
	subs, err := fetchActiveSubs(ctx, client, baseURL, email, token)
	if err != nil {
		return activeSubsResp{}, err
	}

	return selectActiveSub(subs, email)
}

func fetchActiveSubs(ctx context.Context, client *http.Client, baseURL, email, token string) ([]activeSubsResp, error) {
	u := baseURL + "/v1/support/subscribers/" + url.PathEscape(email)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no subscriber found for email %q", email)
	}

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return nil, fmt.Errorf("unexpected status %d: failed to read response body: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result activeSubsListResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, fmt.Errorf("no active subscriptions found for email %q", email)
	}

	return result.Results, nil
}

func selectActiveSub(results []activeSubsResp, email string) (activeSubsResp, error) {
	if len(results) == 1 {
		r := results[0]
		fmt.Printf("Found 1 active subscription for %s (%s)\n\n", r.Email, r.ProductName)
		return r, nil
	}

	fmt.Printf("Found %d active subscriptions matching %q:\n\n", len(results), email)
	fmt.Printf("  %-3s  %-36s  %-36s  %-20s  %s\n", "#", "subscription_id", "order_id", "product", "email")
	fmt.Printf("  %-3s  %-36s  %-36s  %-20s  %s\n",
		"---", strings.Repeat("-", 36), strings.Repeat("-", 36), strings.Repeat("-", 20), strings.Repeat("-", 30))

	for i, r := range results {
		fmt.Printf("  %-3d  %-36s  %-36s  %-20s  %s\n", i+1, r.SubscriptionID, r.OrderID, r.ProductName, r.Email)
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("Select subscription [1-%d]: ", len(results))
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return activeSubsResp{}, fmt.Errorf("reading stdin: %w", err)
			}
			return activeSubsResp{}, fmt.Errorf("no selection made")
		}

		n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
		if err != nil || n < 1 || n > len(results) {
			fmt.Printf("Please enter a number between 1 and %d.\n", len(results))
			continue
		}

		return results[n-1], nil
	}
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
}
