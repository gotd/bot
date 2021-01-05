module github.com/gotd/echobot

go 1.15

require (
	github.com/cockroachdb/pebble v0.0.0-20201130172119-f19faf8529d6
	github.com/gotd/td v0.16.1
	go.uber.org/zap v1.16.0
	golang.org/x/net v0.0.0-20201224014010-6772e930b67b
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

replace github.com/gotd/td v0.16.1 => ../td
