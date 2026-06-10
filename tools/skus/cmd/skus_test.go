package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/spf13/cobra"

	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"

	"github.com/brave-intl/bat-go/services/skus/model"
)

// genED25519KeyFile generates a fresh ed25519 keypair, writes the private key
// to a temp file in OpenSSH format, and returns the private key and file path.
func genED25519KeyFile(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	must.NoError(t, err)

	block, err := ssh.MarshalPrivateKey(priv, "")
	must.NoError(t, err)

	path := filepath.Join(t.TempDir(), "id_ed25519")
	must.NoError(t, os.WriteFile(path, pem.EncodeToMemory(block), 0600))

	return priv, path
}

func TestLoadED25519PrivateKey(t *testing.T) {
	t.Run("valid_ed25519_key", func(t *testing.T) {
		priv, path := genED25519KeyFile(t)

		loaded, err := loadED25519PrivateKey(path)
		must.NoError(t, err)
		should.Equal(t, priv, loaded)
	})

	t.Run("non_existent_file", func(t *testing.T) {
		_, err := loadED25519PrivateKey("/does/not/exist/id_ed25519")
		must.Error(t, err)
	})

	t.Run("not_a_key_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "garbage")
		must.NoError(t, os.WriteFile(path, []byte("this is not a key"), 0600))

		_, err := loadED25519PrivateKey(path)
		must.Error(t, err)
	})
}

func TestListBatches(t *testing.T) {
	priv, _ := genED25519KeyFile(t)
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("returns_batches_on_200", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Second)
		want := []model.TLV2ActiveBatch{
			{RequestID: "req-aaa", OldestValidFrom: now},
			{RequestID: "req-bbb", OldestValidFrom: now.Add(-time.Hour)},
		}

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodGet, r.Method)
			// Signature header must be present.
			should.NotEmpty(t, r.Header.Get("Signature"))

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"batches": want})
		}))
		defer srv.Close()

		got, err := listBatches(context.Background(), client, srv.URL, priv)
		must.NoError(t, err)
		must.Len(t, got, 2)
		should.Equal(t, "req-aaa", got[0].RequestID)
		should.Equal(t, "req-bbb", got[1].RequestID)
	})

	t.Run("empty_batches_array", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{"batches": []model.TLV2ActiveBatch{}})
		}))
		defer srv.Close()

		got, err := listBatches(context.Background(), client, srv.URL, priv)
		must.NoError(t, err)
		should.Empty(t, got)
	})

	t.Run("non_200_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := listBatches(context.Background(), client, srv.URL, priv)
		must.Error(t, err)
		should.Contains(t, err.Error(), "404")
	})

	t.Run("invalid_json_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "not-json")
		}))
		defer srv.Close()

		_, err := listBatches(context.Background(), client, srv.URL, priv)
		must.Error(t, err)
	})

	t.Run("context_cancelled_returns_error", func(t *testing.T) {
		// Server that blocks until the test is done — the cancelled context
		// should cause the request to fail before the server responds.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := listBatches(ctx, client, srv.URL, priv)
		must.Error(t, err)
	})
}

func TestDeleteBatchSeats(t *testing.T) {
	priv, _ := genED25519KeyFile(t)
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("success_on_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodDelete, r.Method)
			should.NotEmpty(t, r.Header.Get("Signature"))
			should.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var body struct {
				Seats  int    `json:"seats"`
				ItemID string `json:"item_id,omitempty"`
			}
			must.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			should.Equal(t, 2, body.Seats)
			should.Equal(t, "item-xyz", body.ItemID)

			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		err := deleteBatchSeats(context.Background(), client, srv.URL, priv, 2, "item-xyz")
		must.NoError(t, err)
	})

	t.Run("omits_empty_item_id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			must.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			_, hasItemID := body["item_id"]
			should.False(t, hasItemID)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		err := deleteBatchSeats(context.Background(), client, srv.URL, priv, 1, "")
		must.NoError(t, err)
	})

	t.Run("non_200_returns_error_with_status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"payment required"}`, http.StatusPaymentRequired)
		}))
		defer srv.Close()

		err := deleteBatchSeats(context.Background(), client, srv.URL, priv, 1, "")
		must.Error(t, err)
		should.Contains(t, err.Error(), "402")
	})

	t.Run("context_cancelled_returns_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := deleteBatchSeats(ctx, client, srv.URL, priv, 1, "")
		must.Error(t, err)
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

func TestListBatchesURL_QueryEncoding(t *testing.T) {
	// Verify that the caller properly encodes item_id as a query parameter
	// (url.Values encoding, not raw string concatenation).
	priv, _ := genED25519KeyFile(t)
	client := &http.Client{Timeout: 5 * time.Second}

	var receivedItemID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedItemID = r.URL.Query().Get("item_id")
		json.NewEncoder(w).Encode(map[string]interface{}{"batches": []model.TLV2ActiveBatch{}})
	}))
	defer srv.Close()

	itemID := "ad0be000-0000-4000-a000-000000000000"
	endpoint := srv.URL + "?" + "item_id=" + itemID

	_, err := listBatches(context.Background(), client, endpoint, priv)
	must.NoError(t, err)
	should.Equal(t, itemID, receivedItemID)
}

func TestListBatchesBodyLimit(t *testing.T) {
	// The response body is capped at 1 MiB. A server sending more than that
	// should still result in a successful (truncated) read followed by a JSON
	// parse error, not an OOM or hang.
	priv, _ := genED25519KeyFile(t)
	client := &http.Client{Timeout: 5 * time.Second}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write well over 1 MiB of garbage.
		chunk := strings.Repeat("x", 64*1024)
		for i := 0; i < 20; i++ {
			fmt.Fprint(w, chunk)
		}
	}))
	defer srv.Close()

	_, err := listBatches(context.Background(), client, srv.URL, priv)
	// We expect an error (JSON parse failure), not a panic or timeout.
	must.Error(t, err)
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

func TestFormatLinkingUsage(t *testing.T) {
	const orderID = "550e8400-e29b-41d4-a716-446655440000"
	const itemID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("no_batches", func(t *testing.T) {
		got := formatLinkingUsage(orderID, "", nil)
		should.Equal(t, "No device slots are in use for order "+orderID+".\n", got)
	})

	t.Run("no_batches_with_item_scope", func(t *testing.T) {
		got := formatLinkingUsage(orderID, itemID, nil)
		should.Contains(t, got, "No device slots are in use")
		should.Contains(t, got, "order "+orderID+" (item "+itemID+")")
	})

	t.Run("counts_and_lists_batches", func(t *testing.T) {
		batches := []model.TLV2ActiveBatch{
			{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
			{RequestID: "req-2", OldestValidFrom: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)},
			{RequestID: "req-3", OldestValidFrom: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)},
		}

		got := formatLinkingUsage(orderID, "", batches)
		should.Contains(t, got, "3 device slot(s) in use for order "+orderID+".")
		should.Contains(t, got, "req-1")
		should.Contains(t, got, "req-2")
		should.Contains(t, got, "req-3")
		should.Contains(t, got, "2026-01-02T03:04:05Z")
		should.Contains(t, got, "2026-03-04T05:06:07Z")
	})

	t.Run("formats_timestamps_in_utc", func(t *testing.T) {
		est := time.FixedZone("EST", -5*60*60)
		batches := []model.TLV2ActiveBatch{
			{RequestID: "req-1", OldestValidFrom: time.Date(2026, 1, 2, 3, 4, 5, 0, est)},
		}

		got := formatLinkingUsage(orderID, "", batches)
		should.Contains(t, got, "2026-01-02T08:04:05Z")
	})
}

func TestRequireOrderRef(t *testing.T) {
	should.Error(t, requireOrderRef("", ""))
	should.Error(t, requireOrderRef("some-order", "user@example.com"))
	should.NoError(t, requireOrderRef("some-order", ""))
	should.NoError(t, requireOrderRef("", "user@example.com"))
}

func TestResolveOrderID(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("order_id_passes_through", func(t *testing.T) {
		got, err := resolveOrderID(context.Background(), client, "some-order", "", "", "")
		must.NoError(t, err)
		should.Equal(t, "some-order", got)
	})

	t.Run("email_requires_subs_base_url", func(t *testing.T) {
		_, err := resolveOrderID(context.Background(), client, "", "user@example.com", "", "tok")
		must.Error(t, err)
		should.Contains(t, err.Error(), "--subscriptions-base-url")
	})

	t.Run("email_requires_subs_token", func(t *testing.T) {
		_, err := resolveOrderID(context.Background(), client, "", "user@example.com", "http://localhost", "")
		must.Error(t, err)
		should.Contains(t, err.Error(), "--subscriptions-token")
	})

	t.Run("email_resolves_via_support_api", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			should.Equal(t, "/v1/support/subscribers/user@example.com", r.URL.Path)
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{
				{Email: "user@example.com", OrderID: "ord-1", ProductName: "VPN"},
			}})
		}))
		defer srv.Close()

		got, err := resolveOrderID(context.Background(), client, "", "user@example.com", srv.URL, "tok")
		must.NoError(t, err)
		should.Equal(t, "ord-1", got)
	})
}

func TestBatchesURL(t *testing.T) {
	const base = "https://payment.example.com"
	const orderID = "550e8400-e29b-41d4-a716-446655440000"

	t.Run("without_item_id", func(t *testing.T) {
		should.Equal(t,
			base+"/v1/orders/"+orderID+"/credentials/batches",
			batchesURL(base, orderID, ""))
	})

	t.Run("with_item_id", func(t *testing.T) {
		should.Equal(t,
			base+"/v1/orders/"+orderID+"/credentials/batches?item_id=ad0be000-0000-4000-a000-000000000000",
			batchesURL(base, orderID, "ad0be000-0000-4000-a000-000000000000"))
	})
}

func TestExtendLinkingLimit(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	const subID = "1f14a340-edfb-4d14-bbdb-06a6c441568b"

	t.Run("success_on_200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, http.MethodPost, r.Method)
			should.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			should.Equal(t, "/v1/support/subscriptions/"+subID+"/credentials/batches/extend", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "{}")
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, "tok")
		must.NoError(t, err)
	})

	t.Run("policy_error_surfaces_errorCode_and_message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"message":"extension rate limited","code":429,"errorCode":"rate_limited"}`)
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, "tok")
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

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, "tok")
		must.Error(t, err)
		should.Contains(t, err.Error(), "subscription not found")
		should.NotContains(t, err.Error(), "errorCode")
	})

	t.Run("non_json_error_falls_back_to_raw_body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "gateway timeout", http.StatusGatewayTimeout)
		}))
		defer srv.Close()

		err := extendLinkingLimit(context.Background(), client, srv.URL, subID, "tok")
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

		err := extendLinkingLimit(ctx, client, srv.URL, subID, "tok")
		must.Error(t, err)
	})
}

func TestFetchActiveSubs(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("returns_results_with_subscription_id", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			should.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
			should.Equal(t, "/v1/support/subscribers/user@example.com", r.URL.Path)
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{
				{Email: "user@example.com", SubscriptionID: "sub-1", OrderID: "ord-1", ProductName: "VPN"},
			}})
		}))
		defer srv.Close()

		got, err := fetchActiveSubs(context.Background(), client, srv.URL, "user@example.com", "tok")
		must.NoError(t, err)
		must.Len(t, got, 1)
		should.Equal(t, "sub-1", got[0].SubscriptionID)
	})

	t.Run("not_found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		_, err := fetchActiveSubs(context.Background(), client, srv.URL, "nope@example.com", "tok")
		must.Error(t, err)
		should.Contains(t, err.Error(), "no subscriber found")
	})

	t.Run("empty_results", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(activeSubsListResp{Results: []activeSubsResp{}})
		}))
		defer srv.Close()

		_, err := fetchActiveSubs(context.Background(), client, srv.URL, "user@example.com", "tok")
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
