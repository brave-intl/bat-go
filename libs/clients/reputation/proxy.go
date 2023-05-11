package reputation

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/getsentry/sentry-go"
	log "github.com/sirupsen/logrus"
)

// ProxyRouter is a reverse proxy to reputation endpoints for client access
func ProxyRouter(
	reputationServer string,
	reputationToken string,
) http.HandlerFunc {
	proxyURL, err := url.Parse(reputationServer)
	if err != nil {
		sentry.CaptureException(err)
		log.Panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(proxyURL)
	proxy.Director = func(req *http.Request) {
		req.Header.Add("X-Forwarded-Host", req.Host)
		req.Header.Add("X-Origin-Host", proxyURL.Host)
		req.Header.Add("Authorization", "Bearer "+reputationToken)
		req.URL.Scheme = proxyURL.Scheme
		req.URL.Host = proxyURL.Host
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
}
