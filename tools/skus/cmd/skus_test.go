package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

// signingKey returns a fresh ed25519 private key for signing test requests.
func signingKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	must.NoError(t, err)

	return priv
}

// signingKeyFile writes a fresh ed25519 private key in OpenSSH format to a temp
// file and returns its path.
func signingKeyFile(t *testing.T) string {
	t.Helper()

	blk, err := ssh.MarshalPrivateKey(signingKey(t), "")
	must.NoError(t, err)

	path := filepath.Join(t.TempDir(), "id_ed25519")
	must.NoError(t, os.WriteFile(path, pem.EncodeToMemory(blk), 0o600))

	return path
}

func TestLoadED25519PrivateKey(t *testing.T) {
	t.Run("valid_ed25519_key", func(t *testing.T) {
		loaded, err := loadED25519PrivateKey(signingKeyFile(t))
		must.NoError(t, err)
		should.NotNil(t, loaded)
	})

	t.Run("non_existent_file", func(t *testing.T) {
		_, err := loadED25519PrivateKey(filepath.Join(t.TempDir(), "nope"))
		must.Error(t, err)
		should.Contains(t, err.Error(), "failed to read key file")
	})

	t.Run("not_a_key_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage")
		must.NoError(t, os.WriteFile(path, []byte("this is not a key"), 0o600))

		_, err := loadED25519PrivateKey(path)
		must.Error(t, err)
		should.Contains(t, err.Error(), "failed to parse SSH private key")
	})

	t.Run("non_ed25519_key_rejected", func(t *testing.T) {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		must.NoError(t, err)

		blk, err := ssh.MarshalPrivateKey(ecKey, "")
		must.NoError(t, err)

		path := filepath.Join(t.TempDir(), "id_ecdsa")
		must.NoError(t, os.WriteFile(path, pem.EncodeToMemory(blk), 0o600))

		_, err = loadED25519PrivateKey(path)
		must.Error(t, err)
		should.Contains(t, err.Error(), "not an ed25519 private key")
	})
}

func TestFlagOrEnv(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("some-flag", "", "")
		return cmd
	}

	t.Run("flag_set_wins", func(t *testing.T) {
		t.Setenv("SOME_TEST_ENV", "from-env")

		cmd := newCmd()
		must.NoError(t, cmd.Flags().Set("some-flag", "from-flag"))

		should.Equal(t, "from-flag", flagOrEnv(cmd, "some-flag", "SOME_TEST_ENV"))
	})

	t.Run("falls_back_to_env", func(t *testing.T) {
		t.Setenv("SOME_TEST_ENV", "from-env")

		should.Equal(t, "from-env", flagOrEnv(newCmd(), "some-flag", "SOME_TEST_ENV"))
	})

	t.Run("empty_when_neither_set", func(t *testing.T) {
		should.Equal(t, "", flagOrEnv(newCmd(), "some-flag", "SOME_TEST_ENV_UNSET"))
	})
}

func TestSubsConn(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "test"}
		addSupportFlags(cmd)
		return cmd
	}

	t.Run("requires_base_url", func(t *testing.T) {
		cmd := newCmd()
		must.NoError(t, cmd.Flags().Set("private-key", signingKeyFile(t)))

		_, _, err := subsConn(cmd)
		must.Error(t, err)
		should.Contains(t, err.Error(), "--subscriptions-base-url")
	})

	t.Run("requires_private_key", func(t *testing.T) {
		cmd := newCmd()
		must.NoError(t, cmd.Flags().Set("subscriptions-base-url", "https://subs.example.com"))

		_, _, err := subsConn(cmd)
		must.Error(t, err)
		should.Contains(t, err.Error(), "--private-key")
	})

	t.Run("trims_trailing_slash_and_loads_key", func(t *testing.T) {
		cmd := newCmd()
		must.NoError(t, cmd.Flags().Set("subscriptions-base-url", "https://subs.example.com/"))
		must.NoError(t, cmd.Flags().Set("private-key", signingKeyFile(t)))

		baseURL, key, err := subsConn(cmd)
		must.NoError(t, err)
		should.Equal(t, "https://subs.example.com", baseURL)
		should.NotNil(t, key)
	})
}

func TestRequireSubRef(t *testing.T) {
	should.Error(t, requireSubRef("", ""))
	should.Error(t, requireSubRef("some-sub", "user@example.com"))
	should.NoError(t, requireSubRef("some-sub", ""))
	should.NoError(t, requireSubRef("", "user@example.com"))
}

func TestResolveSubID(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	key := signingKey(t)

	t.Run("subscription_id_passes_through", func(t *testing.T) {
		got, productName, err := resolveSubID(context.Background(), client, "http://localhost", key, "sub-1", "")
		must.NoError(t, err)
		should.Equal(t, "sub-1", got)
		should.Equal(t, "", productName)
	})

	t.Run("email_resolves_via_support_api", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "/v1/support/subscribers/user@example.com", r.URL.Path)
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{
				{Email: "user@example.com", SubscriptionID: "sub-1", OrderID: "ord-1", ProductName: "VPN"},
			}})
		}))
		defer srv.Close()

		got, productName, err := resolveSubID(context.Background(), client, srv.URL, key, "", "user@example.com")
		must.NoError(t, err)
		should.Equal(t, "sub-1", got)
		should.Equal(t, "VPN", productName)
	})

	t.Run("email_without_subscription_id_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{
				{Email: "user@example.com", OrderID: "ord-1", ProductName: "VPN"},
			}})
		}))
		defer srv.Close()

		_, _, err := resolveSubID(context.Background(), client, srv.URL, key, "", "user@example.com")
		must.Error(t, err)
		should.Contains(t, err.Error(), "no subscription_id")
	})
}

func TestSupportAPIError(t *testing.T) {
	t.Run("error_code_and_message", func(t *testing.T) {
		err := supportAPIError("reset", http.StatusUnprocessableEntity, []byte(`{"message":"seats exceeds active batch count","code":422,"errorCode":"seats_exceeded"}`))
		must.Error(t, err)
		should.Contains(t, err.Error(), "reset failed")
		should.Contains(t, err.Error(), "422")
		should.Contains(t, err.Error(), "seats_exceeded")
		should.Contains(t, err.Error(), "seats exceeds active batch count")
	})

	t.Run("message_only", func(t *testing.T) {
		err := supportAPIError("usage lookup", http.StatusNotFound, []byte(`{"message":"subscription not found","code":404}`))
		must.Error(t, err)
		should.Contains(t, err.Error(), "subscription not found")
		should.NotContains(t, err.Error(), "errorCode")
	})

	t.Run("non_json_falls_back_to_raw_body", func(t *testing.T) {
		err := supportAPIError("extension", http.StatusGatewayTimeout, []byte("gateway timeout"))
		must.Error(t, err)
		should.Contains(t, err.Error(), "504")
		should.Contains(t, err.Error(), "gateway timeout")
	})
}

func TestGetLinkingUsage(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	key := signingKey(t)

	const subID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("success_on_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodGet, r.Method)
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "/v1/support/subscriptions/"+subID+"/credentials/batches", r.URL.Path)

			fmt.Fprint(w, `{"limit":10,"active":2,"batches":[{"request_id":"req-1","oldest_valid_from":"2026-01-02T03:04:05Z"},{"request_id":"req-2","oldest_valid_from":"2026-02-03T04:05:06Z"}]}`)
		}))
		defer srv.Close()

		got, err := getLinkingUsage(context.Background(), client, srv.URL, subID, key)
		must.NoError(t, err)
		should.Equal(t, &linkingUsageResp{
			Limit:  10,
			Active: 2,
			Batches: []linkingBatch{
				{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
				{RequestID: "req-2", OldestValidFrom: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)},
			},
		}, got)
	})

	t.Run("error_surfaces_envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"subscription not found","code":404}`)
		}))
		defer srv.Close()

		_, err := getLinkingUsage(context.Background(), client, srv.URL, subID, key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "usage lookup failed")
		should.Contains(t, err.Error(), "subscription not found")
	})

	t.Run("invalid_json_errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "not-json")
		}))
		defer srv.Close()

		_, err := getLinkingUsage(context.Background(), client, srv.URL, subID, key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "failed to decode usage response")
	})

	t.Run("context_cancelled_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := getLinkingUsage(ctx, client, srv.URL, subID, key)
		must.Error(t, err)
	})
}

func TestResetLinkingSlots(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	key := signingKey(t)

	const subID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("success_on_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodDelete, r.Method)
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "application/json", r.Header.Get("Content-Type"))
			should.Equal(t, "/v1/support/subscriptions/"+subID+"/credentials/batches", r.URL.Path)

			var body struct {
				Seats int `json:"seats"`
			}
			should.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			should.Equal(t, 2, body.Seats)

			fmt.Fprint(w, "{}")
		}))
		defer srv.Close()

		err := resetLinkingSlots(context.Background(), client, srv.URL, subID, key, 2)
		must.NoError(t, err)
	})

	t.Run("policy_error_surfaces_errorCode_and_message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"message":"seats exceeds active batch count","code":422,"errorCode":"seats_exceeded"}`)
		}))
		defer srv.Close()

		err := resetLinkingSlots(context.Background(), client, srv.URL, subID, key, 9)
		must.Error(t, err)
		should.Contains(t, err.Error(), "seats_exceeded")
		should.Contains(t, err.Error(), "seats exceeds active batch count")
		should.Contains(t, err.Error(), "422")
	})

	t.Run("context_cancelled_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := resetLinkingSlots(ctx, client, srv.URL, subID, key, 1)
		must.Error(t, err)
	})
}

func TestExtendLinkingLimit(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	key := signingKey(t)

	const subID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("success_on_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodPost, r.Method)
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "/v1/support/subscriptions/"+subID+"/credentials/batches/extend", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "{}")
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, key)
		must.NoError(t, err)
	})

	t.Run("policy_error_surfaces_errorCode_and_message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"extension rate limited","code":429,"errorCode":"rate_limited"}`)
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "rate_limited")
		should.Contains(t, err.Error(), "extension rate limited")
		should.Contains(t, err.Error(), "429")
	})

	t.Run("error_without_errorCode_uses_message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"subscription not found","code":404}`)
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "subscription not found")
		should.NotContains(t, err.Error(), "errorCode")
	})

	t.Run("non_json_error_falls_back_to_raw_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "504")
		should.Contains(t, err.Error(), "gateway timeout")
	})

	t.Run("context_cancelled_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := extendLinkingLimit(ctx, client, srv.URL, subID, key)
		must.Error(t, err)
	})
}

func TestFormatLinkingUsage(t *testing.T) {
	const subID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("no_batches", func(t *testing.T) {
		got := formatLinkingUsage(subID, "", &linkingUsageResp{Limit: 10})
		should.Equal(t, "0 of 10 device slot(s) in use for subscription "+subID+".\n", got)
	})

	t.Run("with_product_name", func(t *testing.T) {
		got := formatLinkingUsage(subID, "VPN", &linkingUsageResp{Limit: 10, Active: 1, Batches: []linkingBatch{
			{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		}})
		should.Contains(t, got, "1 of 10 device slot(s) in use for subscription "+subID+" (VPN).")
		should.Contains(t, got, "req-1")
	})

	t.Run("counts_and_lists_batches", func(t *testing.T) {
		usage := &linkingUsageResp{
			Limit:  10,
			Active: 3,
			Batches: []linkingBatch{
				{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
				{RequestID: "req-2", OldestValidFrom: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)},
				{RequestID: "req-3", OldestValidFrom: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)},
			},
		}

		got := formatLinkingUsage(subID, "", usage)
		should.Contains(t, got, "3 of 10 device slot(s) in use for subscription "+subID+".")
		should.Contains(t, got, "req-1")
		should.Contains(t, got, "req-2")
		should.Contains(t, got, "req-3")
		should.Contains(t, got, "2026-01-02T03:04:05Z")
		should.Contains(t, got, "2026-03-04T05:06:07Z")
	})

	t.Run("formats_timestamps_in_utc", func(t *testing.T) {
		est := time.FixedZone("EST", -5*60*60)
		usage := &linkingUsageResp{Limit: 10, Active: 1, Batches: []linkingBatch{
			{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, est)},
		}}

		got := formatLinkingUsage(subID, "", usage)
		should.Contains(t, got, "2026-01-02T08:04:05Z")
	})
}

func TestFetchActiveSubs(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	key := signingKey(t)

	t.Run("returns_results_with_subscription_id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "/v1/support/subscribers/user@example.com", r.URL.Path)
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{
				{Email: "user@example.com", SubscriptionID: "sub-1", OrderID: "ord-1", ProductName: "VPN"},
			}})
		}))
		defer srv.Close()

		got, err := fetchActiveSubs(context.Background(), client, srv.URL, "user@example.com", key)
		must.NoError(t, err)
		must.Len(t, got, 1)
		should.Equal(t, "sub-1", got[0].SubscriptionID)
	})

	t.Run("not_found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := fetchActiveSubs(context.Background(), client, srv.URL, "nope@example.com", key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "no subscriber found")
	})

	t.Run("empty_results", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{}})
		}))
		defer srv.Close()

		_, err := fetchActiveSubs(context.Background(), client, srv.URL, "user@example.com", key)
		must.Error(t, err)
		should.Contains(t, err.Error(), "no active subscriptions")
	})
}

func TestSelectActiveSub_single(t *testing.T) {
	// A single result is returned without prompting.
	subs := []activeSubsResp{
		{Email: "user@example.com", SubscriptionID: "sub-1", OrderID: "ord-1", ProductName: "VPN"},
	}

	got, err := selectActiveSub(subs, "user@example.com")
	must.NoError(t, err)
	should.Equal(t, "sub-1", got.SubscriptionID)
}

func TestSelectActiveSub_multiple(t *testing.T) {
	subs := []activeSubsResp{
		{Email: "user@example.com", SubscriptionID: "sub-1", OrderID: "ord-1", ProductName: "VPN"},
		{Email: "user@example.com", SubscriptionID: "sub-2", OrderID: "ord-2", ProductName: "Leo"},
	}

	// selectActiveSub prompts on os.Stdin when there is more than one match;
	// redirect it to a pipe for each case.
	withStdin := func(t *testing.T, input string) (activeSubsResp, error) {
		t.Helper()

		r, w, err := os.Pipe()
		must.NoError(t, err)

		orig := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = orig })

		_, err = fmt.Fprint(w, input)
		must.NoError(t, err)
		w.Close()

		return selectActiveSub(subs, "user@example.com")
	}

	t.Run("selects_by_number", func(t *testing.T) {
		got, err := withStdin(t, "2\n")
		must.NoError(t, err)
		should.Equal(t, "sub-2", got.SubscriptionID)
	})

	t.Run("reprompts_on_out_of_range_then_valid", func(t *testing.T) {
		got, err := withStdin(t, "9\n1\n")
		must.NoError(t, err)
		should.Equal(t, "sub-1", got.SubscriptionID)
	})

	t.Run("reprompts_on_non_numeric_then_valid", func(t *testing.T) {
		got, err := withStdin(t, "abc\n2\n")
		must.NoError(t, err)
		should.Equal(t, "sub-2", got.SubscriptionID)
	})

	t.Run("no_selection_on_eof", func(t *testing.T) {
		_, err := withStdin(t, "")
		must.Error(t, err)
		should.Contains(t, err.Error(), "no selection made")
	})
}

func TestConfirm(t *testing.T) {
	// confirm reads from os.Stdin; redirect it to a pipe for each case.
	withStdin := func(t *testing.T, input string) bool {
		t.Helper()

		r, w, err := os.Pipe()
		must.NoError(t, err)

		orig := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = orig })

		_, err = fmt.Fprint(w, input)
		must.NoError(t, err)
		w.Close()

		return confirm("proceed?")
	}

	t.Run("y_returns_true", func(t *testing.T) {
		should.True(t, withStdin(t, "y\n"))
	})

	t.Run("Y_returns_true", func(t *testing.T) {
		should.True(t, withStdin(t, "Y\n"))
	})

	t.Run("yes_returns_false", func(t *testing.T) {
		// Only bare "y" is accepted; full "yes" is treated as no.
		should.False(t, withStdin(t, "yes\n"))
	})

	t.Run("n_returns_false", func(t *testing.T) {
		should.False(t, withStdin(t, "n\n"))
	})

	t.Run("empty_returns_false", func(t *testing.T) {
		should.False(t, withStdin(t, "\n"))
	})

	t.Run("eof_returns_false", func(t *testing.T) {
		// Closing the write end immediately simulates EOF (e.g. non-interactive pipe).
		should.False(t, withStdin(t, ""))
	})
}
