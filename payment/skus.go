package payment

import (
	"context"
	"fmt"

	appctx "github.com/brave-intl/bat-go/utils/context"
)

// List of all the allowed and whitelisted brave skus

const (
	PROD_USER_WALLET_VOTE       = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOaNAUCBMKm0IaLqxefhvxOtAKB0OfoiPn0NPVfI602J"
	PROD_ANON_CARD_VOTE         = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgrMZm85YYwnmjPXcegy5pBM5C+ZLfrySZfYiSe13yp8o="
	PROD_BRAVE_TOGETHER_FREE    = "MDAyNWxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLnNvZnR3YXJlCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLWZyZWUgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLWZyZWUKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwM2NjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIGZyZWUgdHJpYWwgZm9yIEJyYXZlIFRvZ2V0aGVyCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMmZzaWduYXR1cmUgEyHMOlzoMiUqfKGY/npECUsLh+p0czZJqiRHWcm67x0K"
	PROD_BRAVE_TOGETHER_PAID    = "MDAyMGxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLmNvbQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSAl/eGfP93lrklACcFClNPvkP3Go0HCtfYVQMs5n/NJpgo="
	STAGING_USER_WALLET_VOTE    = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOH4Li+rduCtFOfV8Lfa2o8h4SQjN5CuIwxmeQFjOk4W"
	STAGING_ANON_CARD_VOTE      = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPV/WYY5pXhodMPvsilnrLzNH6MA8nFXwyg0qSWX477M="
	STAGING_BRAVE_TOGETHER_FREE = "MDAyOGxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlc29mdHdhcmUuY29tCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLWZyZWUgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLWZyZWUKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwM2NjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIGZyZWUgdHJpYWwgZm9yIEJyYXZlIFRvZ2V0aGVyCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMmZzaWduYXR1cmUg3cCMuN3F1wVhDvPmV9kA7JuvAgzedifNj2KzUNMLgMIK"
	STAGING_BRAVE_TOGETHER_PAID = "MDAyNWxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLnNvZnR3YXJlCjAwMzBpZGVudGlmaWVyIGJyYXZlLXRvZ2V0aGVyLXBhaWQgc2t1IHRva2VuIHYxCjAwMjBjaWQgc2t1PWJyYXZlLXRvZ2V0aGVyLXBhaWQKMDAxMGNpZCBwcmljZT01CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwNDNjaWQgZGVzY3JpcHRpb249T25lIG1vbnRoIHBhaWQgc3Vic2NyaXB0aW9uIGZvciBCcmF2ZSBUb2dldGhlcgowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIBBaYgRlOpoFKqpcnEzOJFKbLzul3DzLEbQbiJCxd9x3Cg=="
	STAGING_WEBTEST_PJ_SKU_DEMO = "AgEYd2VidGVzdC1wai5oZXJva3VhcHAuY29tAih3ZWJ0ZXN0LXBqLmhlcm9rdWFwcC5jb20gYnJhdmUtdHNoaXJ0IHYxAAIQc2t1PWJyYXZlLXRzaGlydAACCnByaWNlPTAuMjUAAgxjdXJyZW5jeT1CQVQAAgxkZXNjcmlwdGlvbj0AAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiCcJ0zXGbSg+s3vsClkci44QQQTzWJb9UPyJASMVU11jw=="
	DEV_USER_WALLET_VOTE        = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGINiB9dUmpqLyeSEdZ23E4dPXwIBOUNJCFN9d5toIME2M"
	DEV_ANON_CARD_VOTE          = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPpv+Al9jRgVCaR49/AoRrsjQqXGqkwaNfqVka00SJxQ="
	DEV_BRAVE_TOGETHER_FREE     = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1mcmVlIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1mcmVlCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDNjY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBmcmVlIHRyaWFsIGZvciBCcmF2ZSBUb2dldGhlcgowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIGebBXoPnj06tvlJkPEDLp9nfWo6Wfc1Txj6jTlgxjrQCg=="
	DEV_BRAVE_TOGETHER_PAID     = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSDKLJ7NuuzP3KdmTdVnn0dI3JmIfNblQKmY+WBJOqnQJAo="
	DEV_SEARCH_CLOSED_BETA      = "AgEVc2VhcmNoLmJyYXZlLnNvZnR3YXJlAh9zZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBkZW1vAAIWc2t1PXNlYXJjaC1iZXRhLWFjY2VzcwACB3ByaWNlPTAAAgxjdXJyZW5jeT1CQVQAAi1kZXNjcmlwdGlvbj1TZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBhY2Nlc3MAAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiB3uXfAAkNSRQd24jSauRny3VM0BYZ8yOclPTEgPa0xrA=="
	DEV_BRAVE_TALK_FREE         = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDJjaWRlbnRpZmllciBicmF2ZS10YWxrLWZyZWUgc2t1IHRva2VuIHYxCjAwMWNjaWQgc2t1PWJyYXZlLXRhbGstZnJlZQowMDEwY2lkIHByaWNlPTAKMDAxNWNpZCBjdXJyZW5jeT1CQVQKMDAzOGNpZCBkZXNjcmlwdGlvbj1PbmUgbW9udGggZnJlZSB0cmlhbCBmb3IgQnJhdmUgVGFsawowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIDweRDu/2CXxxA8811TLwxIyaB7Pfp92mmrWFn40g+ZVCg=="
	DEV_BRAVE_TALK_PAID         = "MDAyOWxvY2F0aW9uIHRvZ2V0aGVyLmJzZy5icmF2ZS5zb2Z0d2FyZQowMDJjaWRlbnRpZmllciBicmF2ZS10YWxrLXBhaWQgc2t1IHRva2VuIHYxCjAwMWNjaWQgc2t1PWJyYXZlLXRhbGstcGFpZAowMDEwY2lkIHByaWNlPTUKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzZmNpZCBkZXNjcmlwdGlvbj1PbmUgbW9udGggcGFpZCBzdWJzY3JpcHRpb24gZm9yIEJyYXZlIFRhbGsKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAxZmNpZCBwYXltZW50X21ldGhvZHM9c3RyaXBlCjAwMmZzaWduYXR1cmUg7/duqYsSrI0XdNHuN6DGEcJV5k0WQZYt1GZuppQSjOgK"

	DEV_BRAVE_FREE_NOCC_TRIAL      = "MDAyZWxvY2F0aW9uIGZyZWVub2NjdHJpYWwuYnNnLmJyYXZlLnNvZnR3YXJlCjAwMmNpZGVudGlmaWVyIGJyYXZlLWZyZWUtbm9jYyBza3UgdG9rZW4gdjEKMDAxY2NpZCBza3U9YnJhdmUtZnJlZS1ub2NjCjAwNDBjaWQgZGVzY3JpcHRpb249RnJlZSB0cmlhbCAobm8gY2MpIGFjY2VzcyB0byBCcmF2ZSBwcm9kdWN0cwowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJmc2lnbmF0dXJlIBe/wDK8/W4grE3e6QT5UUbS/vOdHpTkmOnAaI3fqZB3Cg=="
	DEV_BRAVE_FREE_PREMIUM_TRIAL   = "MDAzMWxvY2F0aW9uIHByZW1pdW1mcmVldHJpYWwuYnNnLmJyYXZlLnNvZnR3YXJlCjAwMzVpZGVudGlmaWVyIGJyYXZlLWZyZWUtcHJlbWl1bS10cmlhbCBza3UgdG9rZW4gdjEKMDAyNWNpZCBza3U9YnJhdmUtZnJlZS1wcmVtaXVtLXRyaWFsCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQwY2lkIGRlc2NyaXB0aW9uPUZyZWUgdHJpYWwgYWNjZXNzIHRvIEJyYXZlIHByZW1pdW0gcHJvZHVjdHMKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAzOWNpZCBzdHJpcGVfcHJvZHVjdF9pZD1wcmljZV8xSXFnVnBIb2YyMGJwaEc2d2NDQnlyUmYKMDAyZnNpZ25hdHVyZSBQns103SFNaxStMFXJLJMTKFlWBd1jZrmIvzIO4dA4GAo="
	DEV_BRAVE_FREE_UNLIMITED_TRIAL = "MDAzM2xvY2F0aW9uIHVubGltaXRlZGZyZWV0cmlhbC5ic2cuYnJhdmUuc29mdHdhcmUKMDAzN2lkZW50aWZpZXIgYnJhdmUtZnJlZS11bmxpbWl0ZWQtdHJpYWwgc2t1IHRva2VuIHYxCjAwMjdjaWQgc2t1PWJyYXZlLWZyZWUtdW5saW1pdGVkLXRyaWFsCjAwMTBjaWQgcHJpY2U9MAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM4Y2lkIGRlc2NyaXB0aW9uPUZyZWUgdHJpYWwgYWNjZXNzIHRvIEJyYXZlIHByb2R1Y3RzCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWJjaWQgc3RyaXBlX3Byb2R1Y3RfaWQ9CjAwMzljaWQgc3RyaXBlX3Byb2R1Y3RfaWQ9cHJpY2VfMUlxZ1dtSG9mMjBicGhHNkNHWmp6dzdVCjAwMmZzaWduYXR1cmUgUex34NPc/n3gkQtnyepTi7w+0yG37RrQOPzLz2ANZEIK"
	DEV_BRAVE_PREMIUM              = "MDAyOGxvY2F0aW9uIHByZW1pdW0uYnNnLmJyYXZlLnNvZnR3YXJlCjAwMmFpZGVudGlmaWVyIGJyYXZlLXByZW1pdW0gc2t1IHRva2VuIHYxCjAwMWFjaWQgc2t1PWJyYXZlLXByZW1pdW0KMDAxMWNpZCBwcmljZT0xMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM1Y2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIHByb2R1Y3RzCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMzljaWQgc3RyaXBlX3Byb2R1Y3RfaWQ9cHJpY2VfMUlxZ1ZwSG9mMjBicGhHNndjQ0J5clJmCjAwMmZzaWduYXR1cmUgVFSwhkWFIYQnZx77ab0SJzof9suMI0IbDvFrXgu9CGEK"
	DEV_BRAVE_UNLIMITED            = "MDAyYWxvY2F0aW9uIHVubGltaXRlZC5ic2cuYnJhdmUuc29mdHdhcmUKMDAyY2lkZW50aWZpZXIgYnJhdmUtdW5saW1pdGVkIHNrdSB0b2tlbiB2MQowMDFjY2lkIHNrdT1icmF2ZS11bmxpbWl0ZWQKMDAxMWNpZCBwcmljZT0xNQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDM3Y2lkIGRlc2NyaXB0aW9uPVVubGltaXRlZCBhY2Nlc3MgdG8gQnJhdmUgcHJvZHVjdHMKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAzOWNpZCBzdHJpcGVfcHJvZHVjdF9pZD1wcmljZV8xSXFnV21Ib2YyMGJwaEc2Q0daanp3N1UKMDAyZnNpZ25hdHVyZSC6T1tsxPdVFzfye49Sv8lFTzdTsMA2HbsuNO3KCJ2yWgo="
)

var skuMap = map[string]map[string]bool{
	"production": {
		PROD_USER_WALLET_VOTE:    true,
		PROD_ANON_CARD_VOTE:      true,
		PROD_BRAVE_TOGETHER_FREE: true,
		PROD_BRAVE_TOGETHER_PAID: true,
	},
	"staging": {
		STAGING_USER_WALLET_VOTE:    true,
		STAGING_ANON_CARD_VOTE:      true,
		STAGING_BRAVE_TOGETHER_FREE: true,
		STAGING_BRAVE_TOGETHER_PAID: true,
		STAGING_WEBTEST_PJ_SKU_DEMO: true,
	},
	"development": {
		DEV_USER_WALLET_VOTE:           true,
		DEV_ANON_CARD_VOTE:             true,
		DEV_BRAVE_TOGETHER_FREE:        true,
		DEV_BRAVE_TOGETHER_PAID:        true,
		DEV_SEARCH_CLOSED_BETA:         true,
		DEV_BRAVE_TALK_FREE:            true,
		DEV_BRAVE_TALK_PAID:            true,
		DEV_BRAVE_FREE_NOCC_TRIAL:      true,
		DEV_BRAVE_FREE_PREMIUM_TRIAL:   true,
		DEV_BRAVE_FREE_UNLIMITED_TRIAL: true,
		DEV_BRAVE_PREMIUM:              true,
		DEV_BRAVE_UNLIMITED:            true,
	},
}

// temporary, until we can validate macaroon signatures
func validateHardcodedSku(ctx context.Context, sku string) (bool, error) {
	env, err := appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	if err != nil {
		return false, fmt.Errorf("failed to get environment: %w", err)
	}
	valid, ok := skuMap[env][sku]
	return valid && ok, nil
}
