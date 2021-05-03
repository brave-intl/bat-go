package middleware

import (
	"errors"
	"net/http"

	"github.com/brave-intl/bat-go/utils/handlers"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	latencyBuckets = []float64{.25, .5, 1, 2.5, 5, 10}

	inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "in_flight_requests",
		Help: "A gauge of requests currently being served by the wrapped handler.",
	})

	// ConcurrentGoRoutines holds the number of go outines
	ConcurrentGoRoutines = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "concurrent_goroutine",
			Help: "Gauge that holds the current number of goroutines",
		},
		[]string{
			"method",
		},
	)
)

func init() {
	prometheus.MustRegister(inFlightGauge, ConcurrentGoRoutines)
}

// InstrumentRoundTripper instruments an http.RoundTripper to capture metrics like the number
// of active requests, the total number of requests made and latency information
func InstrumentRoundTripper(roundTripper http.RoundTripper, service string) http.RoundTripper {
	inFlightGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "client_in_flight_requests",
		Help:        "A gauge of in-flight requests for the wrapped client.",
		ConstLabels: prometheus.Labels{"service": service},
	})
	// attempt to register, if already registered use the registered one
	if err := prometheus.Register(inFlightGauge); err != nil {
		var are *prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			// already registered.
			inFlightGauge = are.ExistingCollector.(prometheus.Gauge)
		}
	}

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "client_api_requests_total",
			Help:        "A counter for requests from the wrapped client.",
			ConstLabels: prometheus.Labels{"service": service},
		},
		[]string{"code", "method"},
	)
	// if already registered use existing one
	if err := prometheus.Register(counter); err != nil {
		var are *prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			// already registered.
			counter = are.ExistingCollector.(*prometheus.CounterVec)
		}
	}

	// dnsLatencyVec uses custom buckets based on expected dns durations.
	// It has an instance label "event", which is set in the
	// DNSStart and DNSDonehook functions defined in the
	// InstrumentTrace struct below.
	dnsLatencyVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "client_dns_duration_seconds",
			Help:        "Trace dns latency histogram.",
			Buckets:     []float64{.005, .01, .025, .05},
			ConstLabels: prometheus.Labels{"service": service},
		},
		[]string{"event"},
	)
	// if already defined, use existing one
	if err := prometheus.Register(dnsLatencyVec); err != nil {
		var are *prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			// already registered.
			dnsLatencyVec = are.ExistingCollector.(*prometheus.HistogramVec)
		}
	}

	// tlsLatencyVec uses custom buckets based on expected tls durations.
	// It has an instance label "event", which is set in the
	// TLSHandshakeStart and TLSHandshakeDone hook functions defined in the
	// InstrumentTrace struct below.
	tlsLatencyVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "client_tls_duration_seconds",
			Help:        "Trace tls latency histogram.",
			Buckets:     []float64{.05, .1, .25, .5},
			ConstLabels: prometheus.Labels{"service": service},
		},
		[]string{"event"},
	)
	// already registered?
	if err := prometheus.Register(tlsLatencyVec); err != nil {
		var are *prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			// already registered.
			tlsLatencyVec = are.ExistingCollector.(*prometheus.HistogramVec)
		}
	}

	// histVec has no labels, making it a zero-dimensional ObserverVec.
	histVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "client_request_duration_seconds",
			Help:        "A histogram of request latencies.",
			Buckets:     prometheus.DefBuckets,
			ConstLabels: prometheus.Labels{"service": service},
		},
		[]string{},
	)
	// already registered? use existing one
	if err := prometheus.Register(histVec); err != nil {
		var are *prometheus.AlreadyRegisteredError
		if errors.As(err, &are) {
			// already registered.
			histVec = are.ExistingCollector.(*prometheus.HistogramVec)
		}
	}

	// Define functions for the available httptrace.ClientTrace hook
	// functions that we want to instrument.
	trace := &promhttp.InstrumentTrace{
		DNSStart: func(t float64) {
			dnsLatencyVec.WithLabelValues("dns_start")
		},
		DNSDone: func(t float64) {
			dnsLatencyVec.WithLabelValues("dns_done")
		},
		TLSHandshakeStart: func(t float64) {
			tlsLatencyVec.WithLabelValues("tls_handshake_start")
		},
		TLSHandshakeDone: func(t float64) {
			tlsLatencyVec.WithLabelValues("tls_handshake_done")
		},
	}

	// Wrap the specified RoundTripper with middleware.
	return promhttp.InstrumentRoundTripperInFlight(inFlightGauge,
		promhttp.InstrumentRoundTripperCounter(counter,
			promhttp.InstrumentRoundTripperTrace(trace,
				promhttp.InstrumentRoundTripperDuration(histVec, roundTripper),
			),
		),
	)
}

// InstrumentHandlerFunc - helper to wrap up a handler func
func InstrumentHandlerFunc(name string, f handlers.AppHandler) http.HandlerFunc {
	return InstrumentHandler(name, f).ServeHTTP
}

// InstrumentHandler instruments an http.Handler to capture metrics like the number
// the total number of requests served and latency information
func InstrumentHandler(name string, h http.Handler) http.Handler {
	hRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "api_requests_total",
			Help:        "Number of requests per handler.",
			ConstLabels: prometheus.Labels{"handler": name},
		},
		[]string{"code", "method"},
	)

	if err := prometheus.Register(hRequests); err != nil {
		if aerr, ok := err.(prometheus.AlreadyRegisteredError); ok {
			hRequests = aerr.ExistingCollector.(*prometheus.CounterVec)
		} else {
			panic(err)
		}
	}

	hLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "request_duration_seconds",
			Help:        "A histogram of latencies for requests.",
			Buckets:     latencyBuckets,
			ConstLabels: prometheus.Labels{"handler": name},
		},
		[]string{"method"},
	)
	if err := prometheus.Register(hLatency); err != nil {
		if aerr, ok := err.(prometheus.AlreadyRegisteredError); ok {
			hLatency = aerr.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			panic(err)
		}
	}

	return promhttp.InstrumentHandlerInFlight(inFlightGauge,
		promhttp.InstrumentHandlerCounter(hRequests, promhttp.InstrumentHandlerDuration(hLatency, h)),
	)
}

// Metrics returns a http.HandlerFunc for the prometheus /metrics endpoint
func Metrics() http.HandlerFunc {
	return promhttp.Handler().(http.HandlerFunc)
}
