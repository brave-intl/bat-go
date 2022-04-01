module github.com/brave-experiments/ia2-parent/viproxy

go 1.17

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/brave-experiments/viproxy v0.0.0-20220310233634-c31e539539bf
	github.com/mdlayher/vsock v1.1.1
	nitro-shim/utils v0.0.0
)

require (
	github.com/mdlayher/socket v0.2.0 // indirect
	golang.org/x/net v0.0.0-20190503192946-f4e77d36d62c // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.0.0-20220204135822-1c1b9b1eba6a // indirect
)
