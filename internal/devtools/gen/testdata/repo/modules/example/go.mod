module erpora/modules/example

go 1.27

toolchain go1.27.3

replace erpora/sdk => ../../sdk

require (
	erpora/sdk v0.0.0
	github.com/google/uuid v1.6.0
	gorm.io/gorm v1.31.2
)
