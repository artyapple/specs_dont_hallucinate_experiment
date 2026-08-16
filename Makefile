.PHONY: build-evaluator evaluate-bases evaluate-base2-codegen evaluate-base2-direct test-base1-skeleton test-base2-codegen test-base2-direct test-evaluator validate-formal

build-evaluator:
	mkdir -p bin
	cd evaluator && go build -o ../bin/evaluator ./cmd/evaluator

test-evaluator:
	cd evaluator && go test ./...

evaluate-base2-direct:
	mkdir -p results/evaluator-baselines
	cd evaluator && go run ./cmd/evaluator -candidate ../fixtures/base2-direct -output ../results/evaluator-baselines/base2-direct.json

evaluate-base2-codegen:
	mkdir -p results/evaluator-baselines
	cd evaluator && go run ./cmd/evaluator -candidate ../fixtures/base2-codegen -output ../results/evaluator-baselines/base2-codegen.json

evaluate-bases: evaluate-base2-direct evaluate-base2-codegen

test-base1-skeleton:
	./fixtures/test-base1-skeleton.sh

test-base2-direct:
	./fixtures/test-base2-direct.sh

test-base2-codegen:
	./fixtures/test-base2-codegen.sh

validate-formal:
	./fixtures/validate-formal.sh
