BINARY ?= bin/reap

.PHONY: build test lint fmt clean

build:
	go build -o $(BINARY) ./cmd/reap

test:
	go test ./...

lint:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
