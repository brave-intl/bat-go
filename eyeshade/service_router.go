package eyeshade

import (
	"bytes"
	"net/http"
	"os"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/brave-intl/bat-go/middleware"
	appctx "github.com/brave-intl/bat-go/utils/context"
	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/go-chi/chi"
	chiware "github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"github.com/maikelmclauflin/go-boom"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"
)

// RouterStatic holds static routes, not on v1 path
func (service *Service) RouterStatic() chi.Router {
	r := RouterDefunct(false)
	r.Method("GET", "/", handlers.AppHandler(func(w http.ResponseWriter, r *http.Request) *handlers.AppError {
		return handlers.Render(r.Context(), *bytes.NewBufferString("ack."), w, http.StatusOK)
	}))
	return r
}

// HandlerNotFound holds static error routes
func HandlerNotFound(
	w http.ResponseWriter,
	r *http.Request,
) {
	boom.RenderNotFound(w)
}

// RouterV1 holds all of the routes under `/v1/`
func (service *Service) RouterV1() chi.Router {
	r := RouterDefunct(true)
	r.Mount("/accounts", service.RouterAccounts())
	r.Mount("/referrals", service.RouterReferrals())
	r.Mount("/stats", service.RouterStats())
	r.Mount("/publishers", service.RouterSettlements())
	return r
}

// WithRoutes sets up a router using the service
func WithRoutes(service *Service) error {
	service.router.Mount("/", service.RouterStatic())
	service.router.Mount("/v1/", service.RouterV1())
	service.router.NotFound(HandlerNotFound)
	return nil
}

// WithNewRouter creates a new router and attaches it to the service object
func WithNewRouter(service *Service) error {
	service.router = chi.NewRouter()
	return nil
}

// WithMiddleware attaches middleware to a router
func WithMiddleware(service *Service) error {
	logger := service.logger
	ctx := *service.ctx
	buildTime, _ := ctx.Value(appctx.BuildTimeCTXKey).(string)
	commit, _ := ctx.Value(appctx.CommitCTXKey).(string)
	version, _ := ctx.Value(appctx.VersionCTXKey).(string)

	logger.Info().
		Str("version", version).
		Str("commit", commit).
		Str("buildTime", buildTime).
		Msg("server starting up")

	govalidator.SetFieldsRequiredByDefault(true)

	r := service.router

	if os.Getenv("ENV") != "production" {
		r.Use(cors.Handler(cors.Options{
			Debug:            true,
			AllowedOrigins:   []string{"https://confab.bsg.brave.software", "https://together.bsg.brave.software"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "Digest", "Signature"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	// chain should be:
	// id / transfer -> ip -> heartbeat -> request logger / recovery -> token check -> rate limit
	// -> instrumentation -> handler
	r.Use(chiware.RequestID)
	r.Use(middleware.RequestIDTransfer)

	// NOTE: This uses standard fowarding headers, note that this puts implicit trust in the header values
	// provided to us. In particular it uses the first element.
	// Consequently we should consider the request IP as primarily "informational".
	r.Use(chiware.RealIP)

	r.Use(middleware.NewServiceCtx(service))

	r.Use(chiware.Heartbeat("/"))
	// log and recover here
	if logger != nil {
		// Also handles panic recovery
		r.Use(hlog.NewHandler(*logger))
		r.Use(hlog.UserAgentHandler("user_agent"))
		r.Use(hlog.RequestIDHandler("req_id", "Request-Id"))
		r.Use(middleware.RequestLogger(logger))
	}
	// now we have middlewares we want included in logging
	r.Use(chiware.Timeout(15 * time.Second))
	r.Use(middleware.BearerToken)
	if os.Getenv("ENV") == "production" {
		r.Use(middleware.RateLimiter(ctx, 180))
	}
	// use cobra configurations for setting up eyeshade service
	// this way we can have the eyeshade service completely separated from
	// grants service and easily deployable.
	r.Get("/health-check", handlers.HealthCheckHandler(version, buildTime, commit))

	r.Get("/metrics", middleware.Metrics())

	// add profiling flag to enable profiling routes
	if os.Getenv("PPROF_ENABLED") != "" {
		// pprof attaches routes to default serve mux
		// host:6061/debug/pprof/
		go func() {
			log.Error().Err(http.ListenAndServe(":6061", http.DefaultServeMux))
		}()
	}

	return nil
}

// Router returns the router that was last setup using this service
func (service *Service) Router() *chi.Mux {
	return service.router
}
