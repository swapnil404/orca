module github.com/swapnil404/orca/server

go 1.25.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/swapnil404/orca/pkg v0.0.0
	google.golang.org/protobuf v1.36.11
)

require github.com/DATA-DOG/go-sqlmock v1.5.2

replace github.com/swapnil404/orca/pkg => ../pkg
