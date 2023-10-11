module github.com/brave-experiments/ia2-parent/viproxy

go 1.20

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/brave/viproxy v0.1.2
	github.com/mdlayher/vsock v1.2.0
	nitro-shim/utils v0.0.0
)

require (
	github.com/mdlayher/socket v0.4.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
