package payments

var (
	APIBase = map[string]string{
		"":            "http://web.payment-dev.svc.cluster.local",
		"local":       "http://web.payment-dev.svc.cluster.local",
		"development": "http://web.payment-dev.svc.cluster.local",
		"staging":     "http://web.payments-staging.svc.cluster.local",
	}
)
