.PHONY: build check migrate run test validate-formal verify-skeleton

build:
	go build ./...

test:
	go test ./...

validate-formal:
	./scripts/validate-formal.sh

verify-skeleton: validate-formal test build

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/task-service

check:
	./scripts/check.sh
