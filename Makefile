APP_NAME := kubehealer
VERSION := 1.0.0
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
IMAGE_NAME := ghcr.io/olumideadekunle/$(APP_NAME):$(VERSION)

.PHONY: all build test lint docker run-demo clean

all: lint test build

build:
	@echo "==> Building $(APP_NAME) binary..."
	go build -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o bin/$(APP_NAME) ./cmd/kubehealer

test:
	@echo "==> Running Go test suite..."
	go test -v -race -coverprofile=coverage.out ./...

lint:
	@echo "==> Verifying Go formatting and static analysis..."
	go vet ./...

docker:
	@echo "==> Building container image $(IMAGE_NAME)..."
	docker build -t $(IMAGE_NAME) .

run-demo:
	@echo "==> Running Autonomous SRE Chaos Simulation..."
	go run ./cmd/kubehealer -mode=simulate -port=8080

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf bin/ coverage.out
