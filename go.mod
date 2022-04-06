module github.com/afex/hystrix-go

go 1.16

replace github.com/cactus/go-statsd-client => ../go-statsd-client

require (
	github.com/DataDog/datadog-go v4.8.3+incompatible
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/cactus/go-statsd-client v0.0.0-00010101000000-000000000000
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/smartystreets/goconvey v1.7.2
	github.com/stretchr/testify v1.7.1 // indirect
)
