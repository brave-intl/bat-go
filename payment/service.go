package payment

// Service contains datastore
type Service struct {
	datastore Datastore
}

// InitService creates a service using the passed datastore and clients configured from the environment
func InitService(datastore Datastore) (*Service, error) {
	service := &Service{
		datastore: datastore,
	}
	return service, nil
}
