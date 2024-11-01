.PHONY: build
uncloud-dev:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd && \
		scp uncloudd-linux-amd64 spy@192.168.40.243:~/ && \
		ssh spy@192.168.40.243 sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd
		scp uncloudd-linux-amd64 spy@192.168.40.176:~/ && \
		ssh spy@192.168.40.176 sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd

reset-dev:
	ssh spy@192.168.40.243 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"
	ssh spy@192.168.40.176 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"
	ssh ubuntu@152.67.101.197 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/machine/api/pb/*.proto
