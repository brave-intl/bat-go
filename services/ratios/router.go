package ratios

import (
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/go-chi/chi"
)

// Router returns a router with the ratios REST routes registered.
func Router(s *Service) chi.Router {
	r := chi.NewRouter()

	r.Get(
		"/v2/relative/provider/coingecko/{coinIDs}/{vsCurrencies}/{duration}",
		XBraveHeaderInstrumentHandler(
			"GetRelativeHandler",
			middleware.InstrumentHandler("GetRelativeHandler", GetRelativeHandler(s)),
		).ServeHTTP)
	r.Get("/v2/history/coingecko/{coinID}/{vsCurrency}/{duration}", middleware.InstrumentHandler("GetHistoryHandler", GetHistoryHandler(s)).ServeHTTP)
	r.Get("/v2/coinmap/provider/coingecko", middleware.InstrumentHandler("GetMappingHandler", GetMappingHandler(s)).ServeHTTP)
	r.Get("/v2/market/provider/coingecko", middleware.InstrumentHandler("GetCoinMarketsHandler", GetCoinMarketsHandler(s)).ServeHTTP)
	r.Post("/v2/stripe/onramp_sessions", middleware.InstrumentHandler(
		"StripeOnrampSessionsHandler",
		CreateStripeOnrampSessionsHandler(s)).ServeHTTP,
	)

	return r
}
