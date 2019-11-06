package wallet

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore Datastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	return &Service{
		datastore: datastore,
	}, nil
}
