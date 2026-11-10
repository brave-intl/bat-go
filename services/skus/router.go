package skus

import (
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/brave-intl/bat-go/libs/handlers"
	"github.com/brave-intl/bat-go/libs/middleware"
	"github.com/brave-intl/bat-go/services/skus/handler"
)

// OrdersNewRouter returns a router with the orders-new REST routes registered.
func OrdersNewRouter(
	svc *Service,
	authMwr middlewareFn,
	metricsMwr middleware.InstrumentHandlerDef,
	copts cors.Options,
) chi.Router {
	r := chi.NewRouter()

	orderh := handler.NewOrder(svc)

	if os.Getenv("ENV") == "local" {
		corsMwrPost := NewCORSMwr(copts, http.MethodPost)

		r.Method(
			http.MethodOptions,
			"/",
			metricsMwr("CreateOrderNewOptions", corsMwrPost(nil)),
		)

		r.Method(
			http.MethodPost,
			"/",
			metricsMwr(
				"CreateOrderNew",
				corsMwrPost(handlers.AppHandler(orderh.CreateNew)),
			),
		)
	} else {
		r.Method(
			http.MethodPost,
			"/",
			metricsMwr("CreateOrderNew", authMwr(handlers.AppHandler(orderh.CreateNew))),
		)
	}

	corsMwrDelete := NewCORSMwr(copts, http.MethodDelete)

	r.Method(
		http.MethodDelete,
		"/{orderID}",
		metricsMwr(
			"CancelOrderNew",
			corsMwrDelete(authMwr(handlers.AppHandler(orderh.Cancel))),
		),
	)

	corsMwrPatch := NewCORSMwr(copts, http.MethodPatch)

	r.Method(
		http.MethodPatch,
		"/{orderID}/expire",
		metricsMwr(
			"ExpireOrder",
			corsMwrPatch(authMwr(handlers.AppHandler(orderh.Expire))),
		),
	)

	r.Method(
		http.MethodPost,
		"/paid",
		metricsMwr("CreateOrderNewPaid", authMwr(handlers.AppHandler(orderh.CreatePaid))),
	)

	return r
}
