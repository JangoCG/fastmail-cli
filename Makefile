BINARY := $(CURDIR)/bin/fastmail
VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/JangoCG/fastmail-cli/internal/version.Version=$(VERSION) -X github.com/JangoCG/fastmail-cli/internal/version.Commit=$(COMMIT) -X github.com/JangoCG/fastmail-cli/internal/version.Date=$(DATE)
PREFIX ?= /usr/local

.PHONY: help build test lint install clean
help:
	@echo 'Fastmail CLI: make build | test | lint | install | clean'
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/fastmail
test:
	go test -race ./...
lint:
	go vet ./...
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...
	@test -z "$$(gofmt -l cmd internal skills)" || (echo 'Run gofmt on the reported files'; gofmt -l cmd internal skills; exit 1)
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(BINARY) $(DESTDIR)$(PREFIX)/bin/fastmail
clean:
	rm -f $(BINARY)
