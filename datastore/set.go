package datastore

import (
	"context"

	sliceset "github.com/brave-intl/bat-go/utils/set"
)

type setDatastoreKey struct{}

var (
	sliceSets = map[string]*sliceset.SliceSet{}
)

// GetSetDatastore gets the set-like datastore configured in context for collection key
// Defaults to a slice backed datastore
func GetSetDatastore(ctx context.Context, key string) (SetLikeDatastore, error) {
	val := ctx.Value(setDatastoreKey{})
	if val == nil {
		val = ""
	}
	switch val.(string) {
	default:
		fallthrough
	case "slice":
		set, exists := sliceSets[key]
		if !exists {
			tmp := sliceset.NewSliceSet()
			sliceSets[key] = &tmp
			set = &tmp
		}
		return set, nil
	}
}

// SetLikeDatastore is an interface for "set-like" access to a datastore
type SetLikeDatastore interface {
	// Add a single element to the set, return true if newly added
	Add(e string) (bool, error)
	// Cardinality returns the number of elements in the set
	Cardinality() (int, error)
	// Contains returns true if the given item is in the set
	Contains(e string) (bool, error)
	// Close the underlying connection to the datastore
	Close() error
}
