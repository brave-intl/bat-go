package payment

import (
	"context"
	"fmt"

	appctx "github.com/brave-intl/bat-go/utils/context"
)

// skus configured on context:
// {
//    "<sku_value>":"<sku_name>"
// ...
func getSKUsFromContext(ctx context.Context) (map[string]string, bool) {
	skus, ok := ctx.Value(appctx.ValidHardCodedSKUsCTXKey).(map[string]string)
	return skus, ok
}

// IsValidSKU checks to see if the token provided is one that we've previously created
func IsValidSKU(ctx context.Context, sku string) (bool, error) {

	validSKUs, ok := getSKUsFromContext(ctx)
	if !ok {
		return false, fmt.Errorf("valid skus config not available on ctx")
	}

	if _, ok := validSKUs[sku]; ok {
		return true, nil
	}
	return false, nil
}
