module github.com/brave-experiments/ia2-parent/viproxy

go 1.20

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/brave/viproxy v0.1.2
	github.com/mdlayher/vsock v1.2.0
	nitro-shim/utils v0.0.0
)

require (
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/mdlayher/socket v0.4.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sync v0.6.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
)
