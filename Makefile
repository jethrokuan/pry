.PHONY: build test

build:
	go build -o pr-review ./cmd/...

test:
	ginkgo -r -v
