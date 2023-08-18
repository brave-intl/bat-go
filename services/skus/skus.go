package skus

import (
	"context"
	"fmt"

	appctx "github.com/brave-intl/bat-go/libs/context"
)

// List of all the allowed and whitelisted brave skus

const (
	prodUserWalletVote    = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOaNAUCBMKm0IaLqxefhvxOtAKB0OfoiPn0NPVfI602J"
	prodAnonCardVote      = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgrMZm85YYwnmjPXcegy5pBM5C+ZLfrySZfYiSe13yp8o="
	prodBraveTogetherPaid = "MDAyMGxvY2F0aW9uIHRvZ2V0aGVyLmJyYXZlLmNvbQowMDMwaWRlbnRpZmllciBicmF2ZS10b2dldGhlci1wYWlkIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS10b2dldGhlci1wYWlkCjAwMTBjaWQgcHJpY2U9NQowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDQzY2lkIGRlc2NyaXB0aW9uPU9uZSBtb250aCBwYWlkIHN1YnNjcmlwdGlvbiBmb3IgQnJhdmUgVG9nZXRoZXIKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyZnNpZ25hdHVyZSAl/eGfP93lrklACcFClNPvkP3Go0HCtfYVQMs5n/NJpgo="

	prodBraveTalkPremiumTimeLimited             = "MDAxY2xvY2F0aW9uIHRhbGsuYnJhdmUuY29tCjAwNDFpZGVudGlmaWVyIGJyYXZlLXRhbGstcHJlbWl1bS1wcm9kIHRpbWUgbGltaXRlZCBza3UgdG9rZW4gdjEKMDAxZmNpZCBza3U9YnJhdmUtdGFsay1wcmVtaXVtCjAwMTNjaWQgcHJpY2U9Ny4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMxY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFRhbGsKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDEwYmNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9KdzR6UXhkSGtweFNPZSIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSklDcEVCU20xbXRyTjlud0NLdnBZUTQiLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSBO3HtH7rpK5LFD9LIj4m1WGcPjxGO5T3msNCNlySS+QAo="
	prodBraveSearchYearPremiumTimeLimited       = "MDAxZWxvY2F0aW9uIHNlYXJjaC5icmF2ZS5jb20KMDAzMWlkZW50aWZpZXIgYnJhdmUtc2VhcmNoLXByZW1pdW0gc2t1IHRva2VuIHYxCjAwMjFjaWQgc2t1PWJyYXZlLXNlYXJjaC1wcmVtaXVtCjAwMTRjaWQgcHJpY2U9MzAuMDAKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzM2NpZCBkZXNjcmlwdGlvbj1QcmVtaXVtIGFjY2VzcyB0byBCcmF2ZSBTZWFyY2gKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMVkKMDAxZWNpZCBpc3N1YW5jZV9pbnRlcnZhbD1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9LVGx5emVjc3E3ZXZrNiIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSm9vUjhCU20xbXRyTjlubWMydmJUMDciLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUg67IJ+1vENMQjtY96hAj+rfAqPcmxTuxJXzMogrbAK/IK"
	prodBraveSearchPremiumTimeLimited           = "MDAxZWxvY2F0aW9uIHNlYXJjaC5icmF2ZS5jb20KMDAzMWlkZW50aWZpZXIgYnJhdmUtc2VhcmNoLXByZW1pdW0gc2t1IHRva2VuIHYxCjAwMjFjaWQgc2t1PWJyYXZlLXNlYXJjaC1wcmVtaXVtCjAwMTNjaWQgcHJpY2U9My4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMzY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFNlYXJjaAowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTBiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbHl6ZWNzcTdldms2IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29RbkJTbTFtdHJOOW5uMk9NS3BqaiIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLmNvbS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIK0QiErbDD+400vJNO6g2ijcF/5uh7C9RuRvg2q3IFw8Cg=="
	prodBraveFirewallVPNPremiumTimeLimitedV2    = "MDAxYmxvY2F0aW9uIHZwbi5icmF2ZS5jb20KMDAyMWlkZW50aWZpZXIgYnJhdmUtdnBuLXByZW1pdW0KMDAxZWNpZCBza3U9YnJhdmUtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMjZjaWQgZGVzY3JpcHRpb249YnJhdmUtdnBuLXByZW1pdW0KMDAyOGNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkLXYyCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyYmNpZCBlYWNoX2NyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFECjAwMWFjaWQgZXhwaXJlc19hZnRlcj1QMU0KMDAxZmNpZCBpc3N1ZXJfdG9rZW5fYnVmZmVyPTMxCjAwMWZjaWQgaXNzdWVyX3Rva2VuX292ZXJsYXA9MgowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTBiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0xodjhxc1BzbjZXSHJ4IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFMMFZIbUJTbTFtdHJOOW5UNURQbVVaYiIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLmNvbS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIA6wxaFI2HqlTuX+wPorRuUIp4pQv++J1xAMATTnV6kzCg=="
	prodBraveFirewallVPNPremiumTimeLimitedV2BAT = "MDAxYmxvY2F0aW9uIHZwbi5icmF2ZS5jb20KMDAyMWlkZW50aWZpZXIgYnJhdmUtdnBuLXByZW1pdW0KMDAxZWNpZCBza3U9YnJhdmUtdnBuLXByZW1pdW0KMDAxMWNpZCBwcmljZT0xNQowMDE1Y2lkIGN1cnJlbmN5PUJBVAowMDI2Y2lkIGRlc2NyaXB0aW9uPWJyYXZlLXZwbi1wcmVtaXVtCjAwMjhjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZC12MgowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMmJjaWQgZWFjaF9jcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxRAowMDFhY2lkIGV4cGlyZXNfYWZ0ZXI9UDFNCjAwMWZjaWQgaXNzdWVyX3Rva2VuX2J1ZmZlcj0zMQowMDFmY2lkIGlzc3Vlcl90b2tlbl9vdmVybGFwPTIKMDAyNmNpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1yYWRvbQowMGQ0Y2lkIG1ldGFkYXRhPSB7ICJyYWRvbV9wcm9kdWN0X2lkIjogInByb2RfTGh2OHFzUHNuNldIcngiLCAicmFkb21fc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLmNvbS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInJhZG9tX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLmNvbS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUghrNnKGx/369LtfDHdt9u4aorHf9DW2Sq/E9Ou9+jeP8K"

	stagingUserWalletVote   = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOH4Li+rduCtFOfV8Lfa2o8h4SQjN5CuIwxmeQFjOk4W"
	stagingAnonCardVote     = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPV/WYY5pXhodMPvsilnrLzNH6MA8nFXwyg0qSWX477M="
	stagingWebtestPJSKUDemo = "AgEYd2VidGVzdC1wai5oZXJva3VhcHAuY29tAih3ZWJ0ZXN0LXBqLmhlcm9rdWFwcC5jb20gYnJhdmUtdHNoaXJ0IHYxAAIQc2t1PWJyYXZlLXRzaGlydAACCnByaWNlPTAuMjUAAgxjdXJyZW5jeT1CQVQAAgxkZXNjcmlwdGlvbj0AAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiCcJ0zXGbSg+s3vsClkci44QQQTzWJb9UPyJASMVU11jw=="

	stagingBraveSearchPremiumTimeLimited     = "MDAyNmxvY2F0aW9uIHNlYXJjaC5icmF2ZXNvZnR3YXJlLmNvbQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWVjaWQgaXNzdWFuY2VfaW50ZXJ2YWw9UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMWJjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfS1RtNkphWnNzQU5QQnYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpvb1hyQlNtMW10ck45bjNtUklMZVhNIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIKgf59ZBTJMyykzMrRbXaimDbL26csEeNOlcZ0EMUbBsCg=="
	stagingBraveSearchYearPremiumTimeLimited = "MDAyNmxvY2F0aW9uIHNlYXJjaC5icmF2ZXNvZnR3YXJlLmNvbQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxNGNpZCBwcmljZT0zMC4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMzY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFNlYXJjaAowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTFiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbTZKYVpzc0FOUEJ2IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29ZcUJTbTFtdHJOOW54VUJ6ckZwbCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSDc1p+SfPzYa31kyis/j76jiOXm+MxWT0dH8+9LJfNYFwo="

	stagingBraveTalkPremiumTimeLimited             = "MDAyNGxvY2F0aW9uIHRhbGsuYnJhdmVzb2Z0d2FyZS5jb20KMDAyZmlkZW50aWZpZXIgYnJhdmUtdGFsay1wcmVtaXVtIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS10YWxrLXByZW1pdW0KMDAxM2NpZCBwcmljZT03LjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzFjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgVGFsawowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxRAowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTFiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbTRGdGNuaXVUQU9iIiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29XVEJTbTFtdHJOOW5nM0NwRzRtNCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSDtKYgKBLxJ6P0NQ4ZFox1dDVf6yFu4gRsefmiwy7ZN5Qo="
	stagingBraveFirewallVPNPremiumTimeLimited      = "MDAyM2xvY2F0aW9uIHZwbi5icmF2ZXNvZnR3YXJlLmNvbQowMDM3aWRlbnRpZmllciBicmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAxZWNpZCBza3U9YnJhdmUtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMWVjaWQgZGVzY3JpcHRpb249QnJhdmUgVlBOCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMWJjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfTGh2NE9NMWFBUHhmbFkiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUwwVkVoQlNtMW10ck45bkdCNGtaa2ZoIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlID/JefMepasfiYgJmd7seLIrnCYTGHe3u9UHOcVD5ZslCg=="
	stagingBraveFirewallVPNPremiumTimeLimitedV2    = "MDAyM2xvY2F0aW9uIHZwbi5icmF2ZXNvZnR3YXJlLmNvbQowMDIxaWRlbnRpZmllciBicmF2ZS12cG4tcHJlbWl1bQowMDFlY2lkIHNrdT1icmF2ZS12cG4tcHJlbWl1bQowMDEzY2lkIHByaWNlPTkuOTkKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAyNmNpZCBkZXNjcmlwdGlvbj1icmF2ZS12cG4tcHJlbWl1bQowMDI4Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQtdjIKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJiY2lkIGVhY2hfY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMUQKMDAxZmNpZCBpc3N1ZXJfdG9rZW5fYnVmZmVyPTMxCjAwMWZjaWQgaXNzdWVyX3Rva2VuX292ZXJsYXA9MgowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTFiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0xodjRPTTFhQVB4ZmxZIiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFMMFZFaEJTbTFtdHJOOW5HQjRrWmtmaCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSDUdtr4vnEuKViKOGA3uHEdd8FcCuaMITzdFNm0FV6w6go="
	stagingBraveFirewallVPNPremiumTimeLimitedV2BAT = "MDAyM2xvY2F0aW9uIHZwbi5icmF2ZXNvZnR3YXJlLmNvbQowMDIxaWRlbnRpZmllciBicmF2ZS12cG4tcHJlbWl1bQowMDFlY2lkIHNrdT1icmF2ZS12cG4tcHJlbWl1bQowMDExY2lkIHByaWNlPTE1CjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwMjZjaWQgZGVzY3JpcHRpb249YnJhdmUtdnBuLXByZW1pdW0KMDAyOGNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkLXYyCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyYmNpZCBlYWNoX2NyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFECjAwMWZjaWQgaXNzdWVyX3Rva2VuX2J1ZmZlcj0zMQowMDFmY2lkIGlzc3Vlcl90b2tlbl9vdmVybGFwPTIKMDAyNmNpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1yYWRvbQowMGU0Y2lkIG1ldGFkYXRhPSB7ICJyYWRvbV9wcm9kdWN0X2lkIjogInByb2RfTGh2NE9NMWFBUHhmbFkiLCAicmFkb21fc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAicmFkb21fY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIL1gyoBprFu2lcbCvuRoMgPBfDZVFhJ3YYTZQdhWqDYnCg=="

	stagingBrave1MTimeLimitedV2 = "MDAzMGxvY2F0aW9uIHByZW1pdW1mcmVldHJpYWwuYnJhdmVzb2Z0d2FyZS5jb20KMDAyZmlkZW50aWZpZXIgYnJhdmUtZnJlZS0xbS10bHYyIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS1mcmVlLTFtLXRsdjIKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwNDBjaWQgZGVzY3JpcHRpb249RnJlZSB0cmlhbCBhY2Nlc3MgdG8gQnJhdmUgcHJlbWl1bSBwcm9kdWN0cwowMDI4Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQtdjIKMDAyOGNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVBUNjBTCjAwMWZjaWQgaXNzdWVyX3Rva2VuX2J1ZmZlcj0zMAowMDFmY2lkIGlzc3Vlcl90b2tlbl9vdmVybGFwPTEKMDAyZnNpZ25hdHVyZSCCLkg37iCp1uKAYh7MiUQLjILHDWB7tQh1mMXFISCtYgo="
	stagingBrave5MTimeLimitedV2 = "MDAzMGxvY2F0aW9uIHByZW1pdW1mcmVldHJpYWwuYnJhdmVzb2Z0d2FyZS5jb20KMDAyZmlkZW50aWZpZXIgYnJhdmUtZnJlZS01bS10bHYyIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS1mcmVlLTVtLXRsdjIKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwNDBjaWQgZGVzY3JpcHRpb249RnJlZSB0cmlhbCBhY2Nlc3MgdG8gQnJhdmUgcHJlbWl1bSBwcm9kdWN0cwowMDI4Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQtdjIKMDAyOWNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVBUMzAwUwowMDFmY2lkIGlzc3Vlcl90b2tlbl9idWZmZXI9MzAKMDAxZmNpZCBpc3N1ZXJfdG9rZW5fb3ZlcmxhcD0xCjAwMmZzaWduYXR1cmUgBkRRgn1Y5SDmnwnsCfYl3JWpfb/OL5LrFqYezBlc3osK"

	devUserWalletVote    = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGINiB9dUmpqLyeSEdZ23E4dPXwIBOUNJCFN9d5toIME2M"
	devAnonCardVote      = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPpv+Al9jRgVCaR49/AoRrsjQqXGqkwaNfqVka00SJxQ="
	devSearchClosedBeta  = "AgEVc2VhcmNoLmJyYXZlLnNvZnR3YXJlAh9zZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBkZW1vAAIWc2t1PXNlYXJjaC1iZXRhLWFjY2VzcwACB3ByaWNlPTAAAgxjdXJyZW5jeT1CQVQAAi1kZXNjcmlwdGlvbj1TZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBhY2Nlc3MAAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiB3uXfAAkNSRQd24jSauRny3VM0BYZ8yOclPTEgPa0xrA=="
	devFreeTimeLimitedV2 = "MDAzMWxvY2F0aW9uIGZyZWUudGltZS5saW1pdGVkLnYyLmJyYXZlLnNvZnR3YXJlCjAwMjhpZGVudGlmaWVyIGZyZWUtdGltZS1saW1pdGVkLXYyLWRldgowMDI1Y2lkIHNrdT1mcmVlLXRpbWUtbGltaXRlZC12Mi1kZXYKMDAxMGNpZCBwcmljZT0wCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMmRjaWQgZGVzY3JpcHRpb249ZnJlZS10aW1lLWxpbWl0ZWQtdjItZGV2CjAwMjhjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZC12MgowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWZjaWQgaXNzdWVyX3Rva2VuX2J1ZmZlcj0zMAowMDFmY2lkIGlzc3Vlcl90b2tlbl9vdmVybGFwPTEKMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDAyZnNpZ25hdHVyZSAqgung8GCnS0TDch62es768kupFxaEMD1yMSgJX2apdgo="

	devBraveTalkPremiumTimeLimited     = "MDAyMWxvY2F0aW9uIHRhbGsuYnJhdmUuc29mdHdhcmUKMDAyZmlkZW50aWZpZXIgYnJhdmUtdGFsay1wcmVtaXVtIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS10YWxrLXByZW1pdW0KMDAxM2NpZCBwcmljZT03LjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzFjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgVGFsawowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTE1Y2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0psYzIyNGhGdkFNdkVwIiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKODRvTUhvZjIwYnBoRzZOQkFUMnZvciIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSB2eBNwpQ6AtZIy3ZNB8cFB00Fj3pe0YEtEs7O7dkunjAo="
	devBraveSearchPremiumTimeLimited   = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMTVjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfSnpTZXZ5Wk01aUJTcmYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpMVGpISG9mMjBicGhHNjBXWWNQY2drIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIAhy/5h5ssBPusHhT6UPev8JIeKkOJ7l012rVGkxlcDsCg=="
	devBraveSearchPremiumTimeLimitedV2 = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWVjaWQgaXNzdWFuY2VfaW50ZXJ2YWw9UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMTVjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfSnpTZXZ5Wk01aUJTcmYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpMVGpISG9mMjBicGhHNjBXWWNQY2drIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIO/u4ackB8DxBhajNe+5E+encUhHE6A5Zq0JXXTQjLoWCg=="

	devBraveSearchPremiumYearTimeLimited       = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDM2aWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bS15ZWFyIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS1zZWFyY2gtYWRmcmVlCjAwMTRjaWQgcHJpY2U9MzAuMDAKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzM2NpZCBkZXNjcmlwdGlvbj1QcmVtaXVtIGFjY2VzcyB0byBCcmF2ZSBTZWFyY2gKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMVkKMDAxZWNpZCBpc3N1YW5jZV9pbnRlcnZhbD1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9KelNldnlaTTVpQlNyZiIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSm9YdkZIb2YyMGJwaEc2eUg2a1FpUEciLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgfSNU9u0uAbGm1Vi8dKoa9hcK71VeMzGUWq77io6sJgUK"
	devBraveFirewallVPNPremiumTimeLimited      = "MDAyMGxvY2F0aW9uIHZwbi5icmF2ZS5zb2Z0d2FyZQowMDM3aWRlbnRpZmllciBicmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyN2NpZCBza3U9YnJhdmUtZmlyZXdhbGwtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMjljaWQgZGVzY3JpcHRpb249QnJhdmUgRmlyZXdhbGwgKyBWUE4KMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9LMWM4VzNvTTRtVXNHdyIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSk5ZdU5Ib2YyMGJwaEc2QnZnZVlFbnQiLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgZoDg2iXb36IocwS9/MZnvP5Hk2NfAdJ6qMs0kBSyinUK"
	devBraveFirewallVPNPremiumTimeLimitedV2    = "MDAyMGxvY2F0aW9uIHZwbi5icmF2ZS5zb2Z0d2FyZQowMDIxaWRlbnRpZmllciBicmF2ZS12cG4tcHJlbWl1bQowMDI3Y2lkIHNrdT1icmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bQowMDEzY2lkIHByaWNlPTkuOTkKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAyOWNpZCBkZXNjcmlwdGlvbj1CcmF2ZSBGaXJld2FsbCArIFZQTgowMDI4Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQtdjIKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDJiY2lkIGVhY2hfY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMUQKMDAxZmNpZCBpc3N1ZXJfdG9rZW5fYnVmZmVyPTMxCjAwMWZjaWQgaXNzdWVyX3Rva2VuX292ZXJsYXA9MgowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTE1Y2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0sxYzhXM29NNG1Vc0d3IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKTll1TkhvZjIwYnBoRzZCdmdlWUVudCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSCjPGxUzapQKFcpaZiPizs30/xFDUkPTgCkfQN/cB9pnwo="
	devBraveFirewallVPNPremiumTimeLimitedV2BAT = "MDAyMGxvY2F0aW9uIHZwbi5icmF2ZS5zb2Z0d2FyZQowMDIxaWRlbnRpZmllciBicmF2ZS12cG4tcHJlbWl1bQowMDI3Y2lkIHNrdT1icmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bQowMDExY2lkIHByaWNlPTE1CjAwMTVjaWQgY3VycmVuY3k9QkFUCjAwMjljaWQgZGVzY3JpcHRpb249QnJhdmUgRmlyZXdhbGwgKyBWUE4KMDAyOGNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkLXYyCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyYmNpZCBlYWNoX2NyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFECjAwMWZjaWQgaXNzdWVyX3Rva2VuX2J1ZmZlcj0zMQowMDFmY2lkIGlzc3Vlcl90b2tlbl9vdmVybGFwPTIKMDAyNmNpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1yYWRvbQowMGQ2Y2lkIG1ldGFkYXRhPSB7ICJyYWRvbV9wcm9kdWN0X2lkIjogIm5vdCBkZWZpbmVkIiwgInJhZG9tX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInJhZG9tX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSBdGmEv+zPzDso4iNwxXkovgNN+0EMdldX/6aCTMpGveQo="
)

var skuMap = map[string]map[string]bool{
	"production": {
		prodUserWalletVote:                          true,
		prodAnonCardVote:                            true,
		prodBraveTogetherPaid:                       true,
		prodBraveTalkPremiumTimeLimited:             true,
		prodBraveSearchYearPremiumTimeLimited:       true,
		prodBraveSearchPremiumTimeLimited:           true,
		prodBraveFirewallVPNPremiumTimeLimitedV2:    true,
		prodBraveFirewallVPNPremiumTimeLimitedV2BAT: true,
	},
	"staging": {
		stagingUserWalletVote:                          true,
		stagingAnonCardVote:                            true,
		stagingWebtestPJSKUDemo:                        true,
		stagingBraveTalkPremiumTimeLimited:             true,
		stagingBraveSearchPremiumTimeLimited:           true,
		stagingBraveSearchYearPremiumTimeLimited:       true,
		stagingBraveFirewallVPNPremiumTimeLimited:      true,
		stagingBraveFirewallVPNPremiumTimeLimitedV2:    true,
		stagingBraveFirewallVPNPremiumTimeLimitedV2BAT: true,
		stagingBrave1MTimeLimitedV2:                    true,
		stagingBrave5MTimeLimitedV2:                    true,
	},
	"development": {
		devUserWalletVote:                          true,
		devAnonCardVote:                            true,
		devSearchClosedBeta:                        true,
		devBraveTalkPremiumTimeLimited:             true,
		devBraveSearchPremiumTimeLimited:           true,
		devBraveFirewallVPNPremiumTimeLimited:      true,
		devBraveSearchPremiumTimeLimitedV2:         true,
		devBraveSearchPremiumYearTimeLimited:       true,
		devBraveFirewallVPNPremiumTimeLimitedV2:    true,
		devBraveFirewallVPNPremiumTimeLimitedV2BAT: true,
		devFreeTimeLimitedV2:                       true,
	},
}

// temporary, until we can validate macaroon signatures
func validateHardcodedSku(ctx context.Context, sku string) (bool, error) {
	// check sku white list from environment
	whitelistSKUs, ok := ctx.Value(appctx.WhitelistSKUsCTXKey).([]string)
	if ok {
		for _, whitelistSKU := range whitelistSKUs {
			if sku == whitelistSKU {
				return true, nil
			}
		}
	}

	// check hardcoded based on environment (non whitelisted)
	env, err := appctx.GetStringFromContext(ctx, appctx.EnvironmentCTXKey)
	if err != nil {
		return false, fmt.Errorf("failed to get environment: %w", err)
	}
	valid, ok := skuMap[env][sku]
	return valid && ok, nil
}
