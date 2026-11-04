module erpora/modules/fixturesales

go 1.27

toolchain go1.27.3

replace (
	erpora/modules/fixturecore => ../fixturecore
	erpora/sdk => ../../sdk
)

require (
	erpora/modules/fixturecore v0.0.0
	erpora/sdk v0.0.0
)
