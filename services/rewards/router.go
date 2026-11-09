package rewards

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/rewards/handler"
)

// Router returns a router with the rewards REST routes registered.
func Router(s *Service) chi.Router {
	r := chi.NewRouter()

	r.Get("/v1/parameters", middleware.InstrumentHandler("GetParametersHandler", GetParametersHandler(s)).ServeHTTP)
	r.Mount("/v1/cards", cardsRouter(s))

	return r
}

func cardsRouter(svc *Service) chi.Router {
	cardsRouter := chi.NewRouter()

	ch := handler.NewCardsHandler(svc)

	cardsRouter.Method(http.MethodGet, "/", middleware.InstrumentHandler("GetCards", handlers.AppHandler(ch.GetCardsHandler)))

	return cardsRouter
}
