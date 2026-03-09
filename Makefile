.PHONY: build test vet lint clean install

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/mutest ./cmd/mutest

install:
	go install $(LDFLAGS) ./cmd/mutest

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

clean:
	rm -rf bin/
