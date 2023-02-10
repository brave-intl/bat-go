package main

import (
	"net"
	"nitro-shim/utils"
	"os"
	"strconv"
	"strings"

	"github.com/brave-experiments/viproxy"
	"github.com/mdlayher/vsock"
)

const (
	envInAddrs  = "IN_ADDRS"
	envOutAddrs = "OUT_ADDRS"
)

var l = utils.NewLogger("viproxy: ")

func parseAddr(rawAddr string) net.Addr {
	var addr net.Addr
	var err error

	addr, err = net.ResolveTCPAddr("tcp", rawAddr)
	if err == nil {
		return addr
	}

	// We couldn't parse the address, so we must be dealing with AF_VSOCK.  We
	// expect an address like 3:8080.
	fields := strings.Split(rawAddr, ":")
	if len(fields) != 2 {
		l.Fatal("Looks like we're given neither AF_INET nor AF_VSOCK addr.")
	}
	cid, err := strconv.ParseUint(fields[0], 10, 32)
	if err != nil {
		l.Fatal("Couldn't turn CID into integer.")
	}
	port, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil {
		l.Fatal("Couldn't turn port into integer.")
	}

	addr = &vsock.Addr{ContextID: uint32(cid), Port: uint32(port)}

	return addr
}

func main() {
	// E.g.: IN_ADDRS=127.0.0.1:8080,127.0.0.1:8081 OUT_ADDRS=4:8080,4:8081 go run main.go
	inEnv, outEnv := os.Getenv(envInAddrs), os.Getenv(envOutAddrs)
	if inEnv == "" || outEnv == "" {
		l.Fatalf("Environment variables %s and %s not set.", envInAddrs, envOutAddrs)
	}

	rawInAddrs, rawOutAddrs := strings.Split(inEnv, ","), strings.Split(outEnv, ",")
	if len(rawInAddrs) != len(rawOutAddrs) {
		l.Fatalf("%s and %s must contain same number of addresses.", envInAddrs, envOutAddrs)
	}

	var tuples []*viproxy.Tuple
	for i := range rawInAddrs {
		inAddr := parseAddr(rawInAddrs[i])
		outAddr := parseAddr(rawOutAddrs[i])
		tuples = append(tuples, &viproxy.Tuple{InAddr: inAddr, OutAddr: outAddr})
	}

	p := viproxy.NewVIProxy(tuples)
	if err := p.Start(); err != nil {
		l.Fatalf("Failed to start VIProxy: %s", err)
	}
	<-make(chan bool)
}
