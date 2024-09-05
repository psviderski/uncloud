.PHONY: build
uncloudd-dev1:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd && \
		scp uncloudd-linux-amd64 spy@192.168.40.243:~/ && \
		ssh spy@192.168.40.243 sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd
	GOOS=linux GOARCH=amd64 go build -o uncloud-linux-amd64 ./cmd/uncloud && \
		scp uncloud-linux-amd64 spy@192.168.40.243:~/ && \
		ssh spy@192.168.40.243 sudo install ./uncloud-linux-amd64 /usr/local/bin/uncloud

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/machine/api/pb/cluster.proto
