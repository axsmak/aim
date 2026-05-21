VERSION ?= $(shell git describe --tags --match "v[0-9]*" --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test clean install

build:
	go build $(LDFLAGS) -o bin/aiman ./cmd/aim

test:
	go test ./...

clean:
	rm -f bin/aiman

install:
	tmpdir=$$(mktemp -d) && cp packaging/arch/PKGBUILD "$$tmpdir/" && cd "$$tmpdir" && makepkg -si --needed; rm -rf "$$tmpdir"
