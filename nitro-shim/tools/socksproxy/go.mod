module github.com/brave-experiments/go-socks-proxy/v2

go 1.18

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	nitro-shim/utils v0.0.0
)

require golang.org/x/net v0.0.0-20210916014120-12bc252f5db8 // indirect
