package payments

var (
	APIBase = map[string]string{
		"":        "https://web.payments-dev.svc.cluster.local",
		"local":   "https://web.payments-dev.svc.cluster.local",
		"dev":     "https://web.payments-dev.svc.cluster.local",
		"staging": "https://nitro-payments-staging.bsg.brave.com",
	}
)
