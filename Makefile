proto:
	@rm -f pb/rpc_file_service_grpc.pb.go pb/rpc_file_service.pb.go
	@protoc --go-grpc_out=pb --go_out=pb --proto_path=proto proto/rpc_file_service.proto
	@protoc --go-grpc_out=pb --go_out=pb --proto_path=proto --proto_path=$(go list -m -f "{{ .Dir }}" google.golang.org/grpc)/reflection/v1alpha proto/reflection.proto
	@echo "Protobuf files regenerated successfully."

server:
	@go run main.go

evans:
	@evans --host localhost --port 8081 -r

.PHONY: proto evans server
