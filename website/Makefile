WEBSITE_IMAGE ?= ghcr.io/psviderski/uncloud-website:latest

.PHONY: website-image-push
website-image-push:
	docker buildx build --push --platform linux/amd64,linux/arm64 -t "$(WEBSITE_IMAGE)" .
