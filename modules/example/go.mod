module erpora/modules/example

go 1.27

toolchain go1.27.3

replace erpora/sdk => ../../sdk

require (
	erpora/sdk v0.0.0
	github.com/google/uuid v1.6.0
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	golang.org/x/text v0.20.0 // indirect
	gorm.io/gorm v1.31.2 // indirect
)
