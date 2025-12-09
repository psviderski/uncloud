# Server list (user@host format), override via .env or command line
SERVERS ?= spy@192.168.40.243 spy@192.168.40.176

-include .env

FIRST_SERVER := $(firstword $(SERVERS))
OTHER_SERVERS := $(wordlist 2,$(words $(SERVERS)),$(SERVERS))

CORROSION_IMAGE ?= ghcr.io/psviderski/corrosion:latest
UCIND_IMAGE ?= ghcr.io/psviderski/ucind:latest

update-dev:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd
	@for server in $(SERVERS); do \
		rsync -az uncloudd-linux-amd64 $$server:~/ && \
		ssh $$server sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd; \
	done
	rm uncloudd-linux-amd64

update-restart-dev:
	GOOS=linux GOARCH=amd64 go build -o uncloudd-linux-amd64 ./cmd/uncloudd
	@for server in $(SERVERS); do \
		rsync -az uncloudd-linux-amd64 $$server:~/ && \
		ssh $$server "sudo install ./uncloudd-linux-amd64 /usr/local/bin/uncloudd && sudo systemctl restart uncloud"; \
	done
	rm uncloudd-linux-amd64

reset-dev:
	@for server in $(SERVERS); do \
		ssh $$server "sudo systemctl stop uncloud && sudo rm -rf /var/lib/uncloud"; \
	done

demo-reset:
	rm -fv ~/.config/uncloud/config.yaml
	@for server in $(SERVERS); do \
		ssh $$server "AUTO_CONFIRM=true sudo -E uncloud-uninstall"; \
	done

.PHONY: ucind-cluster
ucind-cluster:
	go run ./cmd/ucind cluster rm && go run ./cmd/ucind cluster create -m $(if $(MACHINES_COUNT),$(MACHINES_COUNT),3)

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

.PHONY: mocks
mocks:
	@mockery

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

.PHONY: format fmt
format fmt:
	GOOS=linux golangci-lint fmt

LINT_TARGETS := lint lint-and-fix
.PHONY: $(LINT_TARGETS) _lint
$(LINT_TARGETS): _lint
lint: ARGS=
lint-and-fix: ARGS=--fix
_lint:
# Explicitly set OS to Linux to not skip *_linux.go files when running on macOS.
# Uncloud daemon won't likely support OS other than Linux anytime soon, so for now we can rely on that.
	GOOS=linux golangci-lint run $(ARGS)

.PHONY: cli-docs
cli-docs:
	go run ./cmd/uncloud docs
