package datastore

import (
	"context"

	mapkv "github.com/brave-intl/bat-go/utils/kv"
)

type kvDatastoreKey struct{}

var (
	kv = mapkv.NewUnsafe()
)

// GetKvDatastore returns a key value datastore based on the context
// Defaults to an unsafe map backed datastore
func GetKvDatastore(ctx context.Context) (KvDatastore, error) {
	val := ctx.Value(kvDatastoreKey{})
	if val == nil {
		val = ""
	}
	switch val.(string) {
	default:
		fallthrough
	case "map":
		return kv, nil
	}
}

// KvDatastore is an interface for "key-value" access to a datastore
type KvDatastore interface {
	// Set key to hold the string value with ttl in seconds, returns true if updated successfully.
	// If ttl is negative, the value will not expire.
	// Will only update an existing value if upsert is true.
	// Returns an error if the update failed.
	Set(key string, value string, ttl int, upsert bool) (bool, error)

	// Get value held at key and returns it, error if the key does not exist
	Get(key string) (string, error)

	// Deletes the value held at key, returns true if a value was present
	Delete(key string) (bool, error)

	// Close the underlying connection to the datastore
	Close() error
}
