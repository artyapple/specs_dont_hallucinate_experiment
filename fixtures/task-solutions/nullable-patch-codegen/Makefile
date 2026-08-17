.PHONY: build check generate migrate run test validate-formal verify-generate

build:
	go build ./...

test:
	go test ./...

generate:
	./scripts/generate.sh

verify-generate:
	./scripts/verify-generate.sh

validate-formal:
	./scripts/validate-formal.sh

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/task-service

check:
	./scripts/check.sh
