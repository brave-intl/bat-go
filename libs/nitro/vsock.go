package nitro

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brave-intl/bat-go/libs/closers"
	"github.com/brave-intl/bat-go/libs/logging"
	"github.com/mdlayher/vsock"
)

// NotVsockAddrError indicates that the string does not have the correct structure for a vsock address
type NotVsockAddrError struct{}

func (NotVsockAddrError) Error() string {
	return "addr is not a vsock address"
}

var vsockAddrRegex = regexp.MustCompile(`vm\((\d)\)`)

func parseVsockAddr(addr string) (uint32, uint32, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 1 && len(parts) != 2 {
		return 0, 0, NotVsockAddrError{}
	}

	// default to port 80 if none is specified
	port := uint64(80)
	matches := vsockAddrRegex.FindStringSubmatch(parts[0])
	if len(matches) < 2 {
		return 0, 0, NotVsockAddrError{}
	}
	cid, err := strconv.ParseUint(matches[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("cid must be a valid uint32: %v", err)
	}
	if len(parts) == 2 {
		port, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("port must be a valid uint32: %v", err)
		}
	}

	return uint32(cid), uint32(port), nil
}

// DialContext is a net.Dial wrapper which additionally allows connecting to vsock networks
func DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	logger := logging.Logger(ctx, "nitro.DialContext")
	logger.Info().
		Str("network", fmt.Sprintf("%v", network)).
		Str("addr", fmt.Sprintf("%v", addr)).
		Msg("DialContext")

	cid, port, err := parseVsockAddr(addr)
	if err != nil {
		if _, ok := err.(NotVsockAddrError); ok {
			// fallback to net.Dial
			logger.Error().Err(err).
				Str("cid", fmt.Sprintf("%v", cid)).
				Str("port", fmt.Sprintf("%v", port)).
				Msg("vsock dialing now")
			return net.Dial(network, addr)
		}
		return nil, err
	}

	logger.Info().
		Str("cid", fmt.Sprintf("%v", cid)).
		Str("port", fmt.Sprintf("%v", port)).
		Msg("vsock dialing now")
	return vsock.Dial(cid, port)
}

type proxyClientConfig struct {
	Ctx  context.Context
	Addr string
}

func (p *proxyClientConfig) Proxy(*http.Request) (*url.URL, error) {
	logger := logging.Logger(p.Ctx, "nitro.Proxy")
	logger.Info().
		Str("addr", p.Addr).
		Msg("performing proxy")
	v, err := url.Parse(p.Addr)
	if err != nil {
		logger.Error().Err(err).
			Str("addr", p.Addr).
			Msg("error parsing address")
	}

	return v, err
}

// NewProxyRoundTripper returns an http.RoundTripper which routes outgoing requests through the proxy addr
func NewProxyRoundTripper(ctx context.Context, addr string) http.RoundTripper {
	config := proxyClientConfig{ctx, addr}
	return &http.Transport{
		Proxy:       config.Proxy,
		DialContext: DialContext,
	}
}

// NewReverseProxyServer returns an HTTP server acting as a reverse proxy for the upstream addr specified
func NewReverseProxyServer(
	addr string,
	upstreamURL string,
) (*http.Server, error) {
	proxyURL, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("Could not parse upstreamURL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	proxy.Transport = &http.Transport{
		DialContext: DialContext,
	}
	proxy.Director = func(req *http.Request) {
		req.Header.Add("X-Forwarded-Host", req.Host)
		req.Header.Add("X-Origin-Host", proxyURL.Host)
		req.URL.Scheme = proxyURL.Scheme
		req.URL.Host = proxyURL.Host
	}

	return &http.Server{
		Addr:    addr,
		Handler: proxy,
	}, nil
}

type openProxy struct {
	ConnectTimeout time.Duration
}

// ServeOpenProxy creates a new open HTTP proxy listening on the specified vsock port
func ServeOpenProxy(
	ctx context.Context,
	port uint32,
	connectTimeout time.Duration,
) error {

	logger := logging.Logger(ctx, "nitro")
	logger.Info().Msg("!!!! starting open proxy")

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: openProxy{ConnectTimeout: connectTimeout},
	}

	l, err := vsock.Listen(port)
	if err != nil {
		logger.Error().Err(err).Msg(fmt.Sprintf("listening on vsock port: %v", port))
		return fmt.Errorf("listening on vsock port failed: %v", err)
	}
	defer closers.Panic(ctx, l)

	logger.Info().Msg(fmt.Sprintf("listening on vsock port: %v", port))

	return server.Serve(l)
}

func (op openProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		op.httpProxyHandler(w, r)
	} else {
		op.httpConnectProxyHandler(w, r)
	}
}

func (op openProxy) httpProxyHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer closers.Panic(r.Context(), resp.Body)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (op openProxy) httpConnectProxyHandler(w http.ResponseWriter, r *http.Request) {
	upstream, err := net.DialTimeout("tcp", r.Host, op.ConnectTimeout)
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			http.Error(w, "upstream connect timed out", http.StatusGatewayTimeout)
			return
		}

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go bidirectionalCopy(r.Context(), conn, upstream)
}

func bidirectionalCopy(ctx context.Context, a net.Conn, b net.Conn) {
	defer closers.Panic(ctx, a)
	defer closers.Panic(ctx, b)

	var wg sync.WaitGroup
	// Per https://datatracker.ietf.org/doc/html/rfc7231#section-4.3.6
	//   A tunnel is closed when a tunnel intermediary detects that either
	// side has closed its connection: the intermediary MUST attempt to send
	// any outstanding data that came from the closed side to the other
	// side, close both connections, and then discard any remaining data
	// left undelivered.
	wg.Add(1)
	go syncCopy(&wg, b, a)
	wg.Add(1)
	go syncCopy(&wg, a, b)
	wg.Wait()
}

func syncCopy(wg *sync.WaitGroup, dst io.WriteCloser, src io.ReadCloser) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
}
