# TODO: Makefile is deprecated, add new targets as mise tasks in mise.toml instead.

CORROSION_IMAGE ?= ghcr.io/psviderski/corrosion:latest
UCIND_IMAGE ?= ghcr.io/psviderski/ucind:latest

demo-reset:
	rm -fv ~/.config/uncloud/config.yaml
	ssh ubuntu@152.67.101.197 "AUTO_CONFIRM=true sudo -E uncloud-uninstall && docker rmi caddy:2.9.1"
	ssh root@5.223.45.199 "AUTO_CONFIRM=true sudo -E uncloud-uninstall"
	ssh spy@192.168.40.243 "AUTO_CONFIRM=true sudo -E uncloud-uninstall"

.PHONY: ucind-cluster
ucind-cluster:
	go run ./cmd/ucind cluster rm && go run ./cmd/ucind cluster create -m $(if $(MACHINES_COUNT),$(MACHINES_COUNT),3)

.PHONY: proto
proto:
	mise run proto

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
	go test -shuffle=on -count=1 -v ./...
else
	go test -shuffle=on -race -count=1 -v -run ^$(TEST_NAME)$$ ./...
endif

.PHONY: test-e2e
test-e2e:
	go test -shuffle=on -race -count=1 -v ./test/e2e

.PHONY: test-clean
test-clean:
	@CONTAINERS=$$(docker ps --filter "label=ucind.managed" -q); \
	if [ -n "$$CONTAINERS" ]; then \
		echo "Killing containers..."; \
		docker kill $$CONTAINERS; \
	fi; \
	CONTAINERS_STOPPED=$$(docker ps -a --filter "label=ucind.managed" -q); \
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
