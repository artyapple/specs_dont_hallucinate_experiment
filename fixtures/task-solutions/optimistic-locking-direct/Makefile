.PHONY: build check migrate run test validate-formal

build:
	go build ./...

test:
	go test ./...

validate-formal:
	./scripts/validate-formal.sh

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/task-service

check:
	./scripts/check.sh
