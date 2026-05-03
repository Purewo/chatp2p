.PHONY: dev test fmt

dev:
	go run ./cmd/server

test:
	go test ./...

fmt:
	gofmt -w cmd internal
