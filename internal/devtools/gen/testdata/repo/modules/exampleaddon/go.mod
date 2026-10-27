module erpora/modules/exampleaddon

go 1.27

toolchain go1.27.3

replace (
	erpora/modules/example => ../example
	erpora/sdk => ../../sdk
)

require (
	erpora/modules/example v0.0.0
	erpora/sdk v0.0.0
)
