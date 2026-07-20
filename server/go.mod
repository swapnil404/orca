module github.com/betterorca/betterorca/server

go 1.25.0

require (
	github.com/betterorca/betterorca/pkg v0.0.0
	github.com/gorilla/websocket v1.5.3
	github.com/lib/pq v1.10.9
	google.golang.org/protobuf v1.36.11
)

replace github.com/betterorca/betterorca/pkg => ../pkg
