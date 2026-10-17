include makefile.env

IMAGE ?= barloc/dbt-esync
VERSION ?= latest
CONFIG ?= config.example.yaml

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
	docker build -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

docker-push: docker-build
	docker push $(IMAGE):$(VERSION)
	@if [ "$(VERSION)" != "latest" ]; then docker push $(IMAGE):latest; fi
