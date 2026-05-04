APP=yalinli-api

.PHONY: run test fmt tidy build migrate-up migrate-down

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

tidy:
	go mod tidy

build:
	go build -o bin/$(APP) ./cmd/api

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down
