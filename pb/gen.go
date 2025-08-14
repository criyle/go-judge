// Package pb stores the protobuf implementation for the go-judge gRPC interface
package pb

//go:generate protoc --proto_path=./ --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative judge.proto request.proto response.proto stream_request.proto stream_response.proto file.proto
