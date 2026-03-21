.PHONY: build test

build:
	go build -o "$$HOME/.local/bin/pry" ./cmd/...

test:
	ginkgo -r -v
