module github.com/brave-experiments/ia2-parent/viproxy

go 1.18

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/brave-experiments/viproxy v0.0.0-20220310233634-c31e539539bf
	github.com/brave/viproxy v0.1.2
	github.com/mdlayher/vsock v1.2.0
	nitro-shim/utils v0.0.0
)

require (
	github.com/mdlayher/socket v0.4.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
)
