package wallet

// Service contains datastore and challenge bypass / ledger client connections
type Service struct {
	datastore   Datastore
	roDatastore ReadOnlyDatastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore, roDatastore ReadOnlyDatastore) (*Service, error) {
	return &Service{
		datastore:   datastore,
		roDatastore: roDatastore,
	}, nil
}

// ReadableDatastore returns a read only datastore if available, otherwise a normal datastore
func (service *Service) ReadableDatastore() ReadOnlyDatastore {
	if service.roDatastore != nil {
		return service.roDatastore
	}
	return service.datastore
}
