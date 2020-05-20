package pb

//go:generate protoc --proto_path=./ --go_out=plugins=grpc:. --go_opt=paths=source_relative judge.proto
