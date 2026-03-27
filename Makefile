BINARY_NAME := vial
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -s -w \
	-X github.com/cheesejaguar/vial/internal/cli.version=$(VERSION) \
	-X github.com/cheesejaguar/vial/internal/cli.commit=$(COMMIT) \
	-X github.com/cheesejaguar/vial/internal/cli.date=$(DATE)

.PHONY: build test lint vet clean install dashboard

dashboard:
	cd web && npm install && npm run build
	rm -rf internal/dashboard/static
	cp -r web/build internal/dashboard/static

build: dashboard
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/vial

build-quick:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/vial

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/vial

test:
	go test -race ./internal/...

test-verbose:
	go test -race -v ./internal/...

test-cover:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./internal/...
	go tool cover -html=coverage.txt -o coverage.html

vet:
	go vet ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/ dist/ coverage.txt coverage.html

tidy:
	go mod tidy

man:
	go run ./cmd/vial man --dir man/
