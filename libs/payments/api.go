package payments

var (
	APIBase = map[string]string{
		"":            "http://web.payment-dev.svc.cluster.local",
		"local":       "http://web.payment-dev.svc.cluster.local",
		"development": "http://web.payment-dev.svc.cluster.local",
		"staging":     "https://nitro-payments-staging.bsg.brave.com",
	}
)
