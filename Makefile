BINARY ?= bin/reap
IMAGE  ?= reap:dev

.PHONY: build test lint fmt clean docker

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

docker:
	docker build -t $(IMAGE) .

clean:
	rm -rf bin
