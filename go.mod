module athena-erp

go 1.26.0

require connectrpc.com/connect/v2 v2.0.2

require google.golang.org/protobuf v1.36.11

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/go-sql-driver/mysql v1.9.3 // indirect
)

tool (
	connectrpc.com/connect/v2/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)
