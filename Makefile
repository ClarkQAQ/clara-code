.PHONY: test race vet lint build examples check fmt conformance-openresponses conformance-acp

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

build:
	go build -o clara-code .

examples:
	go build ./examples/...

fmt:
	gofmt -w main.go internal pkg examples

check:
	test -z "$$(gofmt -l main.go internal pkg examples)"
	go test ./...
	go test -race ./...
	go vet ./...
	golangci-lint run ./...
	go build -o clara-code .
	go build ./examples/...

conformance-openresponses:
	bash test/conformance/openresponses.sh

conformance-acp:
	bash test/conformance/acp.sh
