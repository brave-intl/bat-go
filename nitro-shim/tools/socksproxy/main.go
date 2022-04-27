package main

import (
	"context"
	"net"
	"strings"

	utils "nitro-shim/utils"

	socks5 "github.com/armon/go-socks5"
)

const (
	allowed = true
	denied  = false

	envAllowedFQDNs = "SOCKS_PROXY_ALLOWED_FQDNS"
	envAllowedAddrs = "SOCKS_PROXY_ALLOWED_ADDRS"
	envListenAddr   = "SOCKS_PROXY_LISTEN_ADDR"
)

var l = utils.NewLogger("socksproxy: ")

type myRule struct {
	addrs []net.IP
	fqdns []string
}

func logConn(allowed bool, from, to *socks5.AddrSpec) {
	var prefix string
	if allowed {
		prefix = "Allowing"
	} else {
		prefix = "Denying"
	}
	l.Printf("%s connection request from %s:%d (%s) to %s:%d (%s).",
		prefix,
		from.IP, from.Port, from.FQDN,
		to.IP, to.Port, to.FQDN)
}

func (m myRule) Allow(ctx context.Context, req *socks5.Request) (context.Context, bool) {
	for _, addr := range m.addrs {
		if req.DestAddr.IP.Equal(addr) {
			logConn(allowed, req.RemoteAddr, req.DestAddr)
			return ctx, allowed
		}
	}
	for _, fqdn := range m.fqdns {
		if req.DestAddr.FQDN == fqdn {
			logConn(allowed, req.RemoteAddr, req.DestAddr)
			return ctx, allowed
		}
	}
	logConn(denied, req.RemoteAddr, req.DestAddr)
	return ctx, denied
}

func main() {
	var cfg = map[string]string{
		envListenAddr:   "",
		envAllowedAddrs: "",
		envAllowedFQDNs: "",
	}
	if err := utils.ReadConfigFromEnv(cfg); err != nil {
		l.Fatalf("Failed to read config from environment variables: %s", err)
	}

	// allowedAddrs represents the list of IP addresses that the SOCKS server
	// allows connections to.  The list contains our Kafka cluster.
	allowedAddrs := []net.IP{}
	// allowedFQDNs represents the list of FQDNs that the SOCKS server allows
	// connections to.  The list contains Let's Encrypt's domain names.
	allowedFQDNs := []string{}
	for _, strAddr := range strings.Split(cfg[envAllowedAddrs], ",") {
		if addr := net.ParseIP(strings.TrimSpace(strAddr)); addr != nil {
			allowedAddrs = append(allowedAddrs, addr)
		} else {
			l.Printf("ERROR: Failed to parse address %s.", strAddr)
		}
	}
	for _, fqdn := range strings.Split(cfg[envAllowedFQDNs], ",") {
		allowedFQDNs = append(allowedFQDNs, strings.TrimSpace(fqdn))
	}

	conf := &socks5.Config{
		Rules: myRule{
			addrs: allowedAddrs,
			fqdns: allowedFQDNs,
		},
	}
	server, err := socks5.New(conf)
	if err != nil {
		l.Fatalf("Failed to create new SOCKSv5 instance: %s", err)
	}

	// Create SOCKSv5 proxy.
	l.Printf("Starting SOCKSv5 server on %s.", cfg[envListenAddr])
	if err := server.ListenAndServe("tcp", cfg[envListenAddr]); err != nil {
		panic(err)
	}
}
