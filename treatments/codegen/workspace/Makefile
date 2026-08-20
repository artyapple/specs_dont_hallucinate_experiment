.PHONY: build check generate migrate run test validate-formal verify-generate verify-skeleton

build:
	go build ./...

test:
	go test ./...

generate:
	bash ./scripts/generate.sh

verify-generate:
	bash ./scripts/verify-generate.sh

validate-formal:
	./scripts/validate-formal.sh

verify-skeleton: validate-formal test build

migrate:
	go run ./cmd/migrate

run:
	go run ./cmd/task-service

check:
	./scripts/check.sh
