module erpora

go 1.27

toolchain go1.27.3

require (
	erpora/modules/example v0.0.0
	erpora/modules/exampleaddon v0.0.0
	erpora/sdk v0.0.0
	github.com/google/uuid v1.6.0
)

replace (
	erpora/modules/example => ./modules/example
	erpora/modules/exampleaddon => ./modules/exampleaddon
	erpora/sdk => ./sdk
)
