module github.com/rahmadmrhd/erp-go

go 1.27

toolchain go1.27.2

require (
	github.com/caarlos0/env/v11 v11.4.1
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/rahmadmrhd/erp-go/modules/example v0.0.0
	github.com/rahmadmrhd/erp-go/sdk v0.0.0
	golang.org/x/mod v0.42.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9 // indirect
)

replace (
	github.com/rahmadmrhd/erp-go/modules/example => ./modules/example
	github.com/rahmadmrhd/erp-go/sdk => ./sdk
)
