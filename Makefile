.PHONY: fmt-check fmt test vet build check staticcheck vulncheck release-check ci

fmt-check:
	@files="$$(gofmt -l cmd internal)"; test -z "$$files" || { printf '%s\n' "$$files"; exit 1; }

fmt:
	@gofmt -w cmd internal

test:
	@go test ./...

vet:
	@go vet ./...

build:
	@mkdir -p bin
	@go build -o bin/syncai ./cmd/syncai

check: fmt-check vet test build

staticcheck:
	@staticcheck ./...

vulncheck:
	@govulncheck ./...

release-check:
	@goreleaser check

ci: check staticcheck vulncheck release-check
