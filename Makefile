.PHONY: help build clean version major minor build-release fmt vet lint deps

help:
	@echo "quickpip - Available targets:"
	@echo "  build          - Build the binary"
	@echo "  clean          - Clean build artifacts"
	@echo "  version        - Show current version"
	@echo "  major <num>    - Update major version"
	@echo "  minor <num>    - Update minor version"
	@echo "  build-release  - Build release binary with current date"
	@echo "  fmt, vet, lint - Code hygiene"
	@echo "  deps           - Download and verify dependencies"

build:
	@echo "Building quickpip..."
	go build -v -o quickpip ./cmd/quickpip

clean:
	@echo "Cleaning build artifacts..."
	rm -f quickpip
	rm -f quickpip-*.sha256
	rm -f quickpip-v*-linux-amd64

version:
	@./scripts/version.sh current

major:
	@./scripts/version.sh major $(filter-out $@,$(MAKECMDGOALS))

minor:
	@./scripts/version.sh minor $(filter-out $@,$(MAKECMDGOALS))

build-release:
	@./scripts/version.sh build

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet
	@echo "Linting complete"

deps:
	go mod download
	go mod verify

%:
	@:


