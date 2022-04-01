module github.com/brave-experiments/ia2-parent/kafkaproxy

go 1.17

replace nitro-shim/utils v0.0.0 => ../utils

require (
	github.com/brave-experiments/ia2 v0.3.0
	github.com/segmentio/kafka-go v0.4.30
	nitro-shim/utils v0.0.0
)

require (
	github.com/golang/snappy v0.0.4 // indirect
	github.com/klauspost/compress v1.15.1 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pierrec/lz4/v4 v4.1.14 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
)
