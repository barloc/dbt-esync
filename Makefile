include makefile.env

IMAGE ?= barloc/dbt-esync
VERSION ?= latest
CONFIG ?= config.example.yaml
# K8s nodes are amd64; local Mac is often arm64 — always publish both.
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: test build run lint docker-build docker-push

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o build/local/$(SERVICE_NAME) .

run: build
	./build/local/$(SERVICE_NAME) --config $(CONFIG)

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		go vet ./...; \
	fi

docker-build:
	docker buildx build --load --platform linux/amd64 \
		-t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

# Multi-arch push (amd64 for K8s + arm64 for Apple Silicon).
docker-push:
	docker buildx build --platform $(PLATFORMS) --push \
		-t $(IMAGE):$(VERSION) -t $(IMAGE):latest \
		.
