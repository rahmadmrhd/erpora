module github.com/rahmadmrhd/erp-go

go 1.27

toolchain go1.27.2

require (
	github.com/rahmadmrhd/erp-go/modules/example v0.0.0
	github.com/rahmadmrhd/erp-go/sdk v0.0.0
)

require golang.org/x/mod v0.42.0

replace (
	github.com/rahmadmrhd/erp-go/modules/example => ./modules/example
	github.com/rahmadmrhd/erp-go/sdk => ./sdk
)
