package nitro

import (
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

	"github.com/brave-intl/bat-go/utils/closers"
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
	if len(parts) != 1 {
		return 0, 0, NotVsockAddrError{}
	}

	// default to port 80 if none is specified
	port := 80
	cid, err := strconv.Atoi(vsockAddrRegex.FindStringSubmatch(parts[0])[1])
	if err != nil || cid < 0 {
		return 0, 0, fmt.Errorf("cid must be a valid uint32: %v", err)
	}
	if len(parts) == 2 {
		port, err = strconv.Atoi(parts[1])
		if err != nil || port < 0 {
			return 0, 0, fmt.Errorf("port must be a valid uint32: %v", err)
		}
	}
	return uint32(cid), uint32(port), nil
}

// Dial is a net.Dial wrapper which additionally allows connecting to vsock networks
func Dial(network, addr string) (net.Conn, error) {
	cid, port, err := parseVsockAddr(addr)
	if err != nil {
		if _, ok := err.(NotVsockAddrError); ok {
			// fallback to net.Dial
			return net.Dial(network, addr)
		}
		return nil, err
	}

	return vsock.Dial(cid, port)
}

type proxyClientConfig struct {
	Addr string
}

func (p *proxyClientConfig) Proxy(*http.Request) (*url.URL, error) {
	return url.Parse(p.Addr)
}

// NewProxyRoundTripper returns an http.RoundTripper which routes outgoing requests through the proxy addr
func NewProxyRoundTripper(addr string) http.RoundTripper {
	config := proxyClientConfig{addr}
	return &http.Transport{
		Proxy: config.Proxy,
		Dial:  Dial,
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
		Dial: Dial,
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
	port uint32,
	connectTimeout time.Duration,
) error {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: openProxy{ConnectTimeout: connectTimeout},
	}

	l, err := vsock.Listen(port)
	if err != nil {
		return fmt.Errorf("listening on vsock port failed: %v", err)
	}
	defer closers.Panic(l)

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
	defer closers.Panic(resp.Body)
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
	go bidirectionalCopy(conn, upstream)
}

func bidirectionalCopy(a net.Conn, b net.Conn) {
	defer closers.Panic(a)
	defer closers.Panic(b)

	var wg sync.WaitGroup
	// Per https://datatracker.ietf.org/doc/html/rfc7231#section-4.3.6
	//   A tunnel is closed when a tunnel intermediary detects that either
	// side has closed its connection: the intermediary MUST attempt to send
	// any outstanding data that came from the closed side to the other
	// side, close both connections, and then discard any remaining data
	// left undelivered.
	wg.Add(1)
	go syncCopy(&wg, b, a)
	go syncCopy(&wg, a, b)
	wg.Wait()
}

func syncCopy(wg *sync.WaitGroup, dst io.WriteCloser, src io.ReadCloser) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
}
