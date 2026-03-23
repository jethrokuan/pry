.PHONY: build test release

build:
	go build -o "$$HOME/.local/bin/pry" ./cmd/...

test:
	ginkgo -r -v

# Usage: make release VERSION=0.0.1-alpha.2
release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=0.0.1-alpha.2)
endif
	git tag v$(VERSION)
	git push origin v$(VERSION)
