CORROSION_IMAGE ?= ghcr.io/psviderski/corrosion:latest
UCIND_IMAGE ?= ghcr.io/psviderski/ucind:latest
DOCS_IMAGE ?= ghcr.io/psviderski/uncloud-docs:latest

update-dev:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd && \
		scp uncloudd-linux-amd64 spy@192.168.40.243:~/ && \
		ssh spy@192.168.40.243 sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd
		scp uncloudd-linux-amd64 spy@192.168.40.176:~/ && \
		ssh spy@192.168.40.176 sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd && \
		rm uncloudd-linux-amd64
#	GOOS=linux GOARCH=arm64 go build -o uncloudd-linux-arm64 ./cmd/uncloudd && \
#		scp uncloudd-linux-arm64 ubuntu@152.67.101.197:~/ && \
#		ssh ubuntu@152.67.101.197 sudo install ./uncloudd-linux-arm64 /usr/local/bin/uncloudd && \
#		rm uncloudd-linux-arm64

update-restart-dev:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd && \
		scp uncloudd-linux-amd64 spy@192.168.40.243:~/ && \
		ssh spy@192.168.40.243 "sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd && sudo systemctl restart uncloud" && \
		scp uncloudd-linux-amd64 spy@192.168.40.176:~/ && \
		ssh spy@192.168.40.176 "sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd && sudo systemctl restart uncloud" && \
		rm uncloudd-linux-amd64
#	GOOS=linux GOARCH=arm64 go build -o uncloudd-linux-arm64 ./cmd/uncloudd && \
#		scp uncloudd-linux-arm64 ubuntu@152.67.101.197:~/ && \
#		ssh ubuntu@152.67.101.197 "sudo install ./uncloudd-linux-arm64 /usr/local/bin/uncloudd && sudo systemctl restart uncloud" && \
#		rm uncloudd-linux-arm64

reset-dev:
	ssh spy@192.168.40.243 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"
	ssh spy@192.168.40.176 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"
	ssh ubuntu@152.67.101.197 "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"

demo-reset:
	rm -fv ~/.config/uncloud/config.yaml
	ssh ubuntu@152.67.101.197 "AUTO_CONFIRM=true sudo -E uncloud-uninstall && docker rmi caddy:2.9.1"
	ssh root@5.223.45.199 "AUTO_CONFIRM=true sudo -E uncloud-uninstall"
	ssh spy@192.168.40.243 "AUTO_CONFIRM=true sudo -E uncloud-uninstall"

.PHONY: ucind-cluster
ucind-cluster:
	go run ./cmd/ucind cluster rm && go run ./cmd/ucind cluster create -m 3

.PHONY: proto
proto:
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--proto_path=. --proto_path=internal/machine/api/vendor internal/machine/api/pb/*.proto

.PHONY: proto-mise
proto-mise:
	mise exec -- protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--proto_path=. --proto_path=internal/machine/api/vendor internal/machine/api/pb/*.proto

.PHONY: corrosion-image
corrosion-image:
	docker build -t "$(CORROSION_IMAGE)" --target corrosion .

.PHONY: corrosion-multiarch-image-push
corrosion-multiarch-image-push:
	docker buildx build --push --platform linux/amd64,linux/arm64 -t "$(CORROSION_IMAGE)" --target corrosion .

.PHONY: ucind-image
ucind-image:
	docker build -t "$(UCIND_IMAGE)" --target ucind .

.PHONY: ucind-multiarch-image-push
ucind-multiarch-image-push:
	docker buildx build --push --platform linux/amd64,linux/arm64 -t "$(UCIND_IMAGE)" --target ucind .

.PHONY: test
test:
ifeq ($(TEST_NAME),)
	go test -count=1 -v ./...
else
	go test -race -count=1 -v -run ^$(TEST_NAME)$$ ./...
endif

.PHONY: test-e2e
test-e2e:
	go test -race -count=1 -v ./test/e2e

.PHONY: test-clean
test-clean:
	@CONTAINERS=$$(docker ps --filter "name=ucind-test" -q); \
	if [ -n "$$CONTAINERS" ]; then \
		echo "Killing containers..."; \
		docker kill $$CONTAINERS; \
	fi; \
	CONTAINERS_STOPPED=$$(docker ps -a --filter "name=ucind-test" -q); \
	if [ -n "$$CONTAINERS_STOPPED" ]; then \
		echo "Removing stopped containers..."; \
		docker rm $$CONTAINERS_STOPPED; \
	else \
		echo "Nothing to clean."; \
	fi

.PHONY: vet
vet:
	go vet ./...

.PHONY: docs-image-push
docs-image:
	docker buildx build --push --platform linux/amd64,linux/arm64 -t "$(DOCS_IMAGE)" ./docs
