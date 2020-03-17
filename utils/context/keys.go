package context

// CTXKey - a type for context keys
type CTXKey string

const (
	// DatastoreCTXKey - the context key for getting the datastore
	DatastoreCTXKey CTXKey = "datastore"
)
