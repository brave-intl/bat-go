package wallet

import (
	"context"

	"github.com/go-chi/chi/v5"

	"github.com/brave-intl/bat-go/libs/middleware"
)

// RouterConfig carries the environment-derived settings used to build the
// wallet REST router.
type RouterConfig struct {
	// DAppOrigins are the origins allowed by the dapp facing CORS middleware.
	DAppOrigins []string
	// CORSOrigins are the origins allowed by the general CORS middleware.
	CORSOrigins []string
	// Debug enables debug mode for the general CORS middleware.
	Debug bool
	// LinkDrainEnabled controls registration of the wallet link drain routes.
	LinkDrainEnabled bool
	// InstrumentHandler wraps route handlers with metrics instrumentation.
	InstrumentHandler middleware.InstrumentHandlerDef
}

// Router returns a mux with the wallet REST routes registered using cfg.
func Router(ctx context.Context, s *Service, cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()
	RegisterRoutes(ctx, s, r, cfg)

	return r
}
