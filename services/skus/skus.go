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

	prodBraveTalkPremiumTimeLimited       = "MDAxY2xvY2F0aW9uIHRhbGsuYnJhdmUuY29tCjAwNDFpZGVudGlmaWVyIGJyYXZlLXRhbGstcHJlbWl1bS1wcm9kIHRpbWUgbGltaXRlZCBza3UgdG9rZW4gdjEKMDAxZmNpZCBza3U9YnJhdmUtdGFsay1wcmVtaXVtCjAwMTNjaWQgcHJpY2U9Ny4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMxY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFRhbGsKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDEwYmNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9KdzR6UXhkSGtweFNPZSIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSklDcEVCU20xbXRyTjlud0NLdnBZUTQiLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSBO3HtH7rpK5LFD9LIj4m1WGcPjxGO5T3msNCNlySS+QAo="
	prodBraveSearchYearPremiumTimeLimited = "MDAxZWxvY2F0aW9uIHNlYXJjaC5icmF2ZS5jb20KMDAzMWlkZW50aWZpZXIgYnJhdmUtc2VhcmNoLXByZW1pdW0gc2t1IHRva2VuIHYxCjAwMjFjaWQgc2t1PWJyYXZlLXNlYXJjaC1wcmVtaXVtCjAwMTRjaWQgcHJpY2U9MzAuMDAKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzM2NpZCBkZXNjcmlwdGlvbj1QcmVtaXVtIGFjY2VzcyB0byBCcmF2ZSBTZWFyY2gKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMVkKMDAxZWNpZCBpc3N1YW5jZV9pbnRlcnZhbD1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9LVGx5emVjc3E3ZXZrNiIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSm9vUjhCU20xbXRyTjlubWMydmJUMDciLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUg67IJ+1vENMQjtY96hAj+rfAqPcmxTuxJXzMogrbAK/IK"
	prodBraveSearchPremiumTimeLimited     = "MDAxZWxvY2F0aW9uIHNlYXJjaC5icmF2ZS5jb20KMDAzMWlkZW50aWZpZXIgYnJhdmUtc2VhcmNoLXByZW1pdW0gc2t1IHRva2VuIHYxCjAwMjFjaWQgc2t1PWJyYXZlLXNlYXJjaC1wcmVtaXVtCjAwMTNjaWQgcHJpY2U9My4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMzY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFNlYXJjaAowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTBiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbHl6ZWNzcTdldms2IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29RbkJTbTFtdHJOOW5uMk9NS3BqaiIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLmNvbS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIK0QiErbDD+400vJNO6g2ijcF/5uh7C9RuRvg2q3IFw8Cg=="
	stagingUserWalletVote                 = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGIOH4Li+rduCtFOfV8Lfa2o8h4SQjN5CuIwxmeQFjOk4W"
	stagingAnonCardVote                   = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPV/WYY5pXhodMPvsilnrLzNH6MA8nFXwyg0qSWX477M="
	stagingWebtestPJSKUDemo               = "AgEYd2VidGVzdC1wai5oZXJva3VhcHAuY29tAih3ZWJ0ZXN0LXBqLmhlcm9rdWFwcC5jb20gYnJhdmUtdHNoaXJ0IHYxAAIQc2t1PWJyYXZlLXRzaGlydAACCnByaWNlPTAuMjUAAgxjdXJyZW5jeT1CQVQAAgxkZXNjcmlwdGlvbj0AAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiCcJ0zXGbSg+s3vsClkci44QQQTzWJb9UPyJASMVU11jw=="

	stagingBraveSearchPremiumTimeLimited     = "MDAyNmxvY2F0aW9uIHNlYXJjaC5icmF2ZXNvZnR3YXJlLmNvbQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWVjaWQgaXNzdWFuY2VfaW50ZXJ2YWw9UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMWJjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfS1RtNkphWnNzQU5QQnYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpvb1hyQlNtMW10ck45bjNtUklMZVhNIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIKgf59ZBTJMyykzMrRbXaimDbL26csEeNOlcZ0EMUbBsCg=="
	stagingBraveSearchYearPremiumTimeLimited = "MDAyNmxvY2F0aW9uIHNlYXJjaC5icmF2ZXNvZnR3YXJlLmNvbQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxNGNpZCBwcmljZT0zMC4wMAowMDE1Y2lkIGN1cnJlbmN5PVVTRAowMDMzY2lkIGRlc2NyaXB0aW9uPVByZW1pdW0gYWNjZXNzIHRvIEJyYXZlIFNlYXJjaAowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTFiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbTZKYVpzc0FOUEJ2IiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29ZcUJTbTFtdHJOOW54VUJ6ckZwbCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSDc1p+SfPzYa31kyis/j76jiOXm+MxWT0dH8+9LJfNYFwo="

	stagingBraveTalkPremiumTimeLimited        = "MDAyNGxvY2F0aW9uIHRhbGsuYnJhdmVzb2Z0d2FyZS5jb20KMDAyZmlkZW50aWZpZXIgYnJhdmUtdGFsay1wcmVtaXVtIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS10YWxrLXByZW1pdW0KMDAxM2NpZCBwcmljZT03LjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzFjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgVGFsawowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDFlY2lkIGlzc3VhbmNlX2ludGVydmFsPVAxRAowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTFiY2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0tUbTRGdGNuaXVUQU9iIiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKb29XVEJTbTFtdHJOOW5nM0NwRzRtNCIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlc29mdHdhcmUuY29tL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSDtKYgKBLxJ6P0NQ4ZFox1dDVf6yFu4gRsefmiwy7ZN5Qo="
	stagingBraveFirewallVPNPremiumTimeLimited = "MDAyM2xvY2F0aW9uIHZwbi5icmF2ZXNvZnR3YXJlLmNvbQowMDM3aWRlbnRpZmllciBicmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAxZWNpZCBza3U9YnJhdmUtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMWVjaWQgZGVzY3JpcHRpb249QnJhdmUgVlBOCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMWJjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfTGh2NE9NMWFBUHhmbFkiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUwwVkVoQlNtMW10ck45bkdCNGtaa2ZoIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmVzb2Z0d2FyZS5jb20vcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlID/JefMepasfiYgJmd7seLIrnCYTGHe3u9UHOcVD5ZslCg=="

	devUserWalletVote   = "AgEJYnJhdmUuY29tAiNicmF2ZSB1c2VyLXdhbGxldC12b3RlIHNrdSB0b2tlbiB2MQACFHNrdT11c2VyLXdhbGxldC12b3RlAAIKcHJpY2U9MC4yNQACDGN1cnJlbmN5PUJBVAACDGRlc2NyaXB0aW9uPQACGmNyZWRlbnRpYWxfdHlwZT1zaW5nbGUtdXNlAAAGINiB9dUmpqLyeSEdZ23E4dPXwIBOUNJCFN9d5toIME2M"
	devAnonCardVote     = "AgEJYnJhdmUuY29tAiFicmF2ZSBhbm9uLWNhcmQtdm90ZSBza3UgdG9rZW4gdjEAAhJza3U9YW5vbi1jYXJkLXZvdGUAAgpwcmljZT0wLjI1AAIMY3VycmVuY3k9QkFUAAIMZGVzY3JpcHRpb249AAIaY3JlZGVudGlhbF90eXBlPXNpbmdsZS11c2UAAAYgPpv+Al9jRgVCaR49/AoRrsjQqXGqkwaNfqVka00SJxQ="
	devSearchClosedBeta = "AgEVc2VhcmNoLmJyYXZlLnNvZnR3YXJlAh9zZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBkZW1vAAIWc2t1PXNlYXJjaC1iZXRhLWFjY2VzcwACB3ByaWNlPTAAAgxjdXJyZW5jeT1CQVQAAi1kZXNjcmlwdGlvbj1TZWFyY2ggY2xvc2VkIGJldGEgcHJvZ3JhbSBhY2Nlc3MAAhpjcmVkZW50aWFsX3R5cGU9c2luZ2xlLXVzZQAABiB3uXfAAkNSRQd24jSauRny3VM0BYZ8yOclPTEgPa0xrA=="

	devBraveTalkPremiumTimeLimited        = "MDAyMWxvY2F0aW9uIHRhbGsuYnJhdmUuc29mdHdhcmUKMDAyZmlkZW50aWZpZXIgYnJhdmUtdGFsay1wcmVtaXVtIHNrdSB0b2tlbiB2MQowMDFmY2lkIHNrdT1icmF2ZS10YWxrLXByZW1pdW0KMDAxM2NpZCBwcmljZT03LjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzFjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgVGFsawowMDI1Y2lkIGNyZWRlbnRpYWxfdHlwZT10aW1lLWxpbWl0ZWQKMDAyNmNpZCBjcmVkZW50aWFsX3ZhbGlkX2R1cmF0aW9uPVAxTQowMDI3Y2lkIGFsbG93ZWRfcGF5bWVudF9tZXRob2RzPXN0cmlwZQowMTE1Y2lkIG1ldGFkYXRhPSB7ICJzdHJpcGVfcHJvZHVjdF9pZCI6ICJwcm9kX0psYzIyNGhGdkFNdkVwIiwgInN0cmlwZV9pdGVtX2lkIjogInByaWNlXzFKODRvTUhvZjIwYnBoRzZOQkFUMnZvciIsICJzdHJpcGVfc3VjY2Vzc191cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL2FjY291bnQvP2ludGVudD1wcm92aXNpb24iLCAic3RyaXBlX2NhbmNlbF91cmkiOiAiaHR0cHM6Ly9hY2NvdW50LmJyYXZlLnNvZnR3YXJlL3BsYW5zLz9pbnRlbnQ9Y2hlY2tvdXQiIH0KMDAyZnNpZ25hdHVyZSB2eBNwpQ6AtZIy3ZNB8cFB00Fj3pe0YEtEs7O7dkunjAo="
	devBraveSearchPremiumTimeLimited      = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMTVjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfSnpTZXZ5Wk01aUJTcmYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpMVGpISG9mMjBicGhHNjBXWWNQY2drIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIAhy/5h5ssBPusHhT6UPev8JIeKkOJ7l012rVGkxlcDsCg=="
	devBraveSearchPremiumTimeLimitedV2    = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDMxaWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyMWNpZCBza3U9YnJhdmUtc2VhcmNoLXByZW1pdW0KMDAxM2NpZCBwcmljZT0zLjAwCjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMzNjaWQgZGVzY3JpcHRpb249UHJlbWl1bSBhY2Nlc3MgdG8gQnJhdmUgU2VhcmNoCjAwMjVjaWQgY3JlZGVudGlhbF90eXBlPXRpbWUtbGltaXRlZAowMDI2Y2lkIGNyZWRlbnRpYWxfdmFsaWRfZHVyYXRpb249UDFNCjAwMWVjaWQgaXNzdWFuY2VfaW50ZXJ2YWw9UDFNCjAwMjdjaWQgYWxsb3dlZF9wYXltZW50X21ldGhvZHM9c3RyaXBlCjAxMTVjaWQgbWV0YWRhdGE9IHsgInN0cmlwZV9wcm9kdWN0X2lkIjogInByb2RfSnpTZXZ5Wk01aUJTcmYiLCAic3RyaXBlX2l0ZW1faWQiOiAicHJpY2VfMUpMVGpISG9mMjBicGhHNjBXWWNQY2drIiwgInN0cmlwZV9zdWNjZXNzX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvYWNjb3VudC8/aW50ZW50PXByb3Zpc2lvbiIsICJzdHJpcGVfY2FuY2VsX3VyaSI6ICJodHRwczovL2FjY291bnQuYnJhdmUuc29mdHdhcmUvcGxhbnMvP2ludGVudD1jaGVja291dCIgfQowMDJmc2lnbmF0dXJlIO/u4ackB8DxBhajNe+5E+encUhHE6A5Zq0JXXTQjLoWCg=="
	devBraveSearchPremiumYearTimeLimited  = "MDAyM2xvY2F0aW9uIHNlYXJjaC5icmF2ZS5zb2Z0d2FyZQowMDM2aWRlbnRpZmllciBicmF2ZS1zZWFyY2gtcHJlbWl1bS15ZWFyIHNrdSB0b2tlbiB2MQowMDIwY2lkIHNrdT1icmF2ZS1zZWFyY2gtYWRmcmVlCjAwMTRjaWQgcHJpY2U9MzAuMDAKMDAxNWNpZCBjdXJyZW5jeT1VU0QKMDAzM2NpZCBkZXNjcmlwdGlvbj1QcmVtaXVtIGFjY2VzcyB0byBCcmF2ZSBTZWFyY2gKMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMVkKMDAxZWNpZCBpc3N1YW5jZV9pbnRlcnZhbD1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9KelNldnlaTTVpQlNyZiIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSm9YdkZIb2YyMGJwaEc2eUg2a1FpUEciLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgfSNU9u0uAbGm1Vi8dKoa9hcK71VeMzGUWq77io6sJgUK"
	devBraveFirewallVPNPremiumTimeLimited = "MDAyMGxvY2F0aW9uIHZwbi5icmF2ZS5zb2Z0d2FyZQowMDM3aWRlbnRpZmllciBicmF2ZS1maXJld2FsbC12cG4tcHJlbWl1bSBza3UgdG9rZW4gdjEKMDAyN2NpZCBza3U9YnJhdmUtZmlyZXdhbGwtdnBuLXByZW1pdW0KMDAxM2NpZCBwcmljZT05Ljk5CjAwMTVjaWQgY3VycmVuY3k9VVNECjAwMjljaWQgZGVzY3JpcHRpb249QnJhdmUgRmlyZXdhbGwgKyBWUE4KMDAyNWNpZCBjcmVkZW50aWFsX3R5cGU9dGltZS1saW1pdGVkCjAwMjZjaWQgY3JlZGVudGlhbF92YWxpZF9kdXJhdGlvbj1QMU0KMDAyN2NpZCBhbGxvd2VkX3BheW1lbnRfbWV0aG9kcz1zdHJpcGUKMDExNWNpZCBtZXRhZGF0YT0geyAic3RyaXBlX3Byb2R1Y3RfaWQiOiAicHJvZF9LMWM4VzNvTTRtVXNHdyIsICJzdHJpcGVfaXRlbV9pZCI6ICJwcmljZV8xSk5ZdU5Ib2YyMGJwaEc2QnZnZVlFbnQiLCAic3RyaXBlX3N1Y2Nlc3NfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9hY2NvdW50Lz9pbnRlbnQ9cHJvdmlzaW9uIiwgInN0cmlwZV9jYW5jZWxfdXJpIjogImh0dHBzOi8vYWNjb3VudC5icmF2ZS5zb2Z0d2FyZS9wbGFucy8/aW50ZW50PWNoZWNrb3V0IiB9CjAwMmZzaWduYXR1cmUgZoDg2iXb36IocwS9/MZnvP5Hk2NfAdJ6qMs0kBSyinUK"
)

var skuMap = map[string]map[string]bool{
	"production": {
		prodUserWalletVote:                    true,
		prodAnonCardVote:                      true,
		prodBraveTogetherPaid:                 true,
		prodBraveTalkPremiumTimeLimited:       true,
		prodBraveSearchYearPremiumTimeLimited: true,
		prodBraveSearchPremiumTimeLimited:     true,
	},
	"staging": {
		stagingUserWalletVote:                     true,
		stagingAnonCardVote:                       true,
		stagingWebtestPJSKUDemo:                   true,
		stagingBraveTalkPremiumTimeLimited:        true,
		stagingBraveSearchPremiumTimeLimited:      true,
		stagingBraveSearchYearPremiumTimeLimited:  true,
		stagingBraveFirewallVPNPremiumTimeLimited: true,
	},
	"development": {
		devUserWalletVote:                     true,
		devAnonCardVote:                       true,
		devSearchClosedBeta:                   true,
		devBraveTalkPremiumTimeLimited:        true,
		devBraveSearchPremiumTimeLimited:      true,
		devBraveFirewallVPNPremiumTimeLimited: true,
		devBraveSearchPremiumTimeLimitedV2:    true,
		devBraveSearchPremiumYearTimeLimited:  true,
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
