package skus

// SubmitRecieptResponseV1 - response from submit reciept
type SubmitRecieptResponseV1 struct {
	ExternalID string `json:"externalId"`
	Vendor     string `json:"vendor"`
}
