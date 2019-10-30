package reputation

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	raven "github.com/getsentry/raven-go"
	log "github.com/sirupsen/logrus"
)

// ProxyRouter is a reverse proxy to reputation endpoints for client access
func ProxyRouter() http.HandlerFunc {
	reputationServer := os.Getenv("REPUTATION_SERVER")
	reputationToken := os.Getenv("REPUTATION_TOKEN")
	if len(reputationServer) == 0 || len(reputationToken) == 0 {
		panic("Must set REPUTATION_SERVER and REPUTATION_TOKEN")
	}

	proxyURL, err := url.Parse(reputationServer)
	if err != nil {
		raven.CaptureErrorAndWait(err, nil)
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
