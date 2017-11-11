package datastore

import (
	"context"
	"errors"
	"github.com/brave-intl/bat-go/utils"
	sliceset "github.com/brave-intl/bat-go/utils/set"
)

var (
	sliceSets = map[string]sliceset.SliceSet{}
)

func GetSetDatastore(ctx context.Context, key string) (SetLikeDatastore, error) {
	switch ctx.Value("datastore.set").(string) {
	case "redis":
		conn := utils.GetRedisConn(ctx)
		set := GetRedisSet(conn, key)
		return &set, nil
	case "slice":
		set, exists := sliceSets[key]
		if !exists {
			set = sliceset.NewSliceSet()
			sliceSets[key] = set
		}
		return &set, nil
	default:
		return nil, errors.New("No such supported set-like datastore")
	}
}

// An interface for "set-like" access to a datastore
type SetLikeDatastore interface {
	// Add a single element to the set, return true if newly added
	Add(e string) (bool, error)

	// Returns the number of elements in the set
	Cardinality() (int, error)

	// Returns whether the given item is in the set
	Contains(e string) (bool, error)

	// Close the underlying connection to the datastore
	Close() error
}
