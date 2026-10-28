module erpora/modules/exampleaddon

go 1.27

toolchain go1.27.3

require (
	erpora/modules/example v0.0.0
	erpora/sdk v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace (
	erpora/modules/example => ../example
	erpora/sdk => ../../sdk
)
