module erpora/modules/core

go 1.27

toolchain go1.27.3

replace erpora/sdk => ../../sdk

require erpora/sdk v0.0.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
