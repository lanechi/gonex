module github.com/lanechi/gonex/contrib/redislog

go 1.26.0

require (
	github.com/lanechi/gonex v0.1.4
	github.com/redis/go-redis/v9 v9.7.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.28.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/lanechi/gonex => ../..
