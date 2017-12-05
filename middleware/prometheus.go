package middleware

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	latencyBuckets = []float64{.25, .5, 1, 2.5, 5, 10}

	inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "in_flight_requests",
		Help: "A gauge of requests currently being served by the wrapped handler.",
	})

	counter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "api_requests_total",
			Help:        "Number of requests per handler.",
			ConstLabels: prometheus.Labels{"handler": "all"},
		},
		[]string{"code", "method"},
	)

	histogramOpts = prometheus.HistogramOpts{
		Name:        "request_duration_seconds",
		Help:        "A histogram of latencies for requests.",
		Buckets:     latencyBuckets,
		ConstLabels: prometheus.Labels{"handler": "all"},
	}
	allVec = prometheus.NewHistogramVec(
		histogramOpts,
		[]string{"method"},
	)

	// responseSize has no labels, making it a zero-dimensional
	// ObserverVec.
	responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "response_size_bytes",
			Help:    "A histogram of response sizes for requests.",
			Buckets: []float64{200, 500, 900, 1500},
		},
		[]string{},
	)
)

func InstrumentRoundTripper(roundTripper http.RoundTripper, service string) http.RoundTripper {
	inFlightGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "client_in_flight_requests",
		Help:        "A gauge of in-flight requests for the wrapped client.",
		ConstLabels: prometheus.Labels{"service": service},
	})

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "client_api_requests_total",
			Help:        "A counter for requests from the wrapped client.",
			ConstLabels: prometheus.Labels{"service": service},
		},
		[]string{"code", "method"},
	)

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

	// Register all of the metrics in the standard registry.
	prometheus.MustRegister(counter, tlsLatencyVec, dnsLatencyVec, histVec, inFlightGauge)

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

func InstrumentHandler(name string, h http.HandlerFunc) http.HandlerFunc {
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

	return promhttp.InstrumentHandlerCounter(hRequests, promhttp.InstrumentHandlerDuration(hLatency, h))
}

func Instrument(next http.Handler) http.Handler {
	return promhttp.InstrumentHandlerInFlight(inFlightGauge,
		promhttp.InstrumentHandlerCounter(counter,
			promhttp.InstrumentHandlerDuration(allVec,
				promhttp.InstrumentHandlerResponseSize(responseSize, next),
			),
		),
	)
}

func Metrics() http.HandlerFunc {
	// Register all of the metrics in the standard registry.
	prometheus.MustRegister(inFlightGauge, counter, allVec, responseSize)

	return promhttp.Handler().(http.HandlerFunc)
}
