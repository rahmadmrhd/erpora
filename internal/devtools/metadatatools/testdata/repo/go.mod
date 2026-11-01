module erpora

go 1.27

toolchain go1.27.3

require (
	erpora/modules/fixturecore v0.0.0
	erpora/modules/fixturesales v0.0.0
	erpora/sdk v0.0.0
)

replace (
	erpora/modules/fixturecore => ./modules/fixturecore
	erpora/modules/fixturesales => ./modules/fixturesales
	erpora/sdk => ./sdk
)
