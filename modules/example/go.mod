module erpora/modules/example

go 1.27

toolchain go1.27.3

replace erpora/sdk => ../../sdk

require (
	erpora/sdk v0.0.0
	github.com/google/uuid v1.6.0
	gorm.io/gorm v1.31.2
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
