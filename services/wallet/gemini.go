package wallet

import (
	"context"
	"strings"

	"github.com/brave-intl/bat-go/libs/clients/gemini"
	appctx "github.com/brave-intl/bat-go/libs/context"
	"github.com/brave-intl/bat-go/libs/custodian"
	errorutils "github.com/brave-intl/bat-go/libs/errors"
)

type geminix struct {
	docTypes []string
}

func newGeminix(docTypePrecedence ...string) *geminix {
	return &geminix{docTypes: docTypePrecedence}
}

// GetIssuingCountry returns the issuing country for the provided gemini.ValidatedAccount.
//
// GetIssuingCountry will primarily try to use the valid documents attached to the account,
// if no valid documents are associated with the account and the fallback param is true then the account
// country code will be used.
//
// GetIssuingCountry returns an empty string when no accepted document types are associated with the account.
func (x *geminix) GetIssuingCountry(acc gemini.ValidatedAccount, fallback bool) string {
	var issuingCountry string

	if fallback {
		issuingCountry = acc.CountryCode
	}

	if len(acc.ValidDocuments) > 0 {
		issuingCountry = countryForDocByPrecedence(x.docTypes, acc.ValidDocuments)
	}

	return issuingCountry
}

func countryForDocByPrecedence(precedence []string, docs []gemini.ValidDocument) string {
	var result string

	for _, pdoc := range precedence {
		for _, vdoc := range docs {
			if strings.EqualFold(pdoc, vdoc.Type) {
				return strings.ToUpper(vdoc.IssuingCountry)
			}
		}
	}

	return result
}

func (x *geminix) IsRegionAvailable(ctx context.Context, issuingCountry string, custodianRegions custodian.Regions) error {
	if useCustodianRegions, ok := ctx.Value(appctx.UseCustodianRegionsCTXKey).(bool); ok && useCustodianRegions {
		allowed := custodianRegions.Gemini.Verdict(issuingCountry)
		if !allowed {
			return errorutils.ErrInvalidCountry
		}
	} else {
		if blacklist, ok := ctx.Value(appctx.BlacklistedCountryCodesCTXKey).([]string); ok {
			for _, v := range blacklist {
				if strings.EqualFold(issuingCountry, v) {
					return errorutils.ErrInvalidCountry
				}
			}
		}
	}
	return nil
}
