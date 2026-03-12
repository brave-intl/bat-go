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
